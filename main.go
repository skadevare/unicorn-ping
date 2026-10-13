package main

import (
	"bytes"
	"context"
	"encoding/binary"
	"errors"
	"flag"
	"fmt"
	"net"
	"os"
	"os/signal"
	"runtime"
	"sort"
	"strings"
	"sync"
	"syscall"
	"time"
)

const version = "1.0.0"

type options struct {
	count      int
	countSet   bool
	timeoutSet bool
	forever    bool
	timeout    time.Duration
	interval   time.Duration
	size       int
	mute       bool
	noColor    bool
	noBanner   bool
	workers    int
	showAll    bool
}

func main() {
	o, target := parseArgs()
	useColor = !o.noColor && os.Getenv("NO_COLOR") == "" && enableTerminal()

	p, err := newPinger()
	if err != nil {
		fmt.Fprintln(os.Stderr, c(red, "💔 "+err.Error()))
		os.Exit(2)
	}

	snd := newSoundPlayer(!o.mute)
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)

	if !o.noBanner {
		printBanner()
	}

	var code int
	if strings.Contains(target, "/") {
		code = sweep(ctx, p, target, o, snd)
	} else {
		code = pingHost(ctx, p, target, o, snd)
	}

	stop()
	snd.Close()
	os.Exit(code)
}

func parseArgs() (options, string) {
	var o options
	fs := flag.NewFlagSet("unicornping", flag.ExitOnError)
	fs.IntVar(&o.count, "n", 4, "number of pings to send")
	fs.IntVar(&o.count, "c", 4, "same as -n")
	fs.BoolVar(&o.forever, "t", false, "ping until stopped with Ctrl+C")
	timeoutMs := fs.Int("w", 4000, "timeout in milliseconds to wait for each pong (subnet mode default: 1000)")
	intervalMs := fs.Int("i", 1000, "milliseconds between pings")
	fs.IntVar(&o.size, "l", 32, "payload size in bytes")
	fs.BoolVar(&o.mute, "mute", false, "don't play the pong sound")
	fs.BoolVar(&o.mute, "nosound", false, "same as -mute")
	fs.BoolVar(&o.noColor, "nocolor", false, "disable rainbows (plain output)")
	fs.BoolVar(&o.noBanner, "nobanner", false, "don't show the unicorn")
	fs.IntVar(&o.workers, "workers", 128, "subnet mode: hosts pinged in parallel")
	fs.BoolVar(&o.showAll, "all", false, "subnet mode: also list hosts that stayed silent")

	fs.Usage = func() {
		out := fs.Output()
		fmt.Fprintf(out, "🦄 unicornping %s - like ping, but every reply is a pong!\n\n", version)
		fmt.Fprintln(out, "Usage:")
		fmt.Fprintln(out, "  unicornping [options] <host | ip | subnet/cidr>")
		fmt.Fprintln(out, "\nExamples:")
		fmt.Fprintln(out, "  unicornping 8.8.8.8")
		fmt.Fprintln(out, "  unicornping google.com -t")
		fmt.Fprintln(out, "  unicornping -mute 1.1.1.1 -n 10")
		fmt.Fprintln(out, "  unicornping 192.168.1.0/24")
		fmt.Fprintln(out, "\nOptions:")
		fs.PrintDefaults()
		fmt.Fprintln(out, "\nSet UNICORNPING_MUTE=1 to mute the pong sound permanently.")
	}

	if err := fs.Parse(reorderArgs(fs, os.Args[1:])); err != nil {
		os.Exit(2)
	}
	fs.Visit(func(f *flag.Flag) {
		if f.Name == "n" || f.Name == "c" {
			o.countSet = true
		}
		if f.Name == "w" {
			o.timeoutSet = true
		}
	})
	if v := os.Getenv("UNICORNPING_MUTE"); v != "" && v != "0" {
		o.mute = true
	}

	if fs.NArg() != 1 {
		fs.Usage()
		os.Exit(2)
	}
	if o.count < 1 || *timeoutMs < 1 || *intervalMs < 0 || o.size < 0 || o.size > 65500 || o.workers < 1 {
		fmt.Fprintln(os.Stderr, "💔 invalid option value (count>=1, timeout>=1, 0<=size<=65500, workers>=1)")
		os.Exit(2)
	}
	o.timeout = time.Duration(*timeoutMs) * time.Millisecond
	o.interval = time.Duration(*intervalMs) * time.Millisecond
	return o, fs.Arg(0)
}

// reorderArgs moves flags in front of the target so both
// "unicornping -t host" and "unicornping host -t" work, like ping.exe.
// On Windows, "/t"-style switches are accepted too.
func reorderArgs(fs *flag.FlagSet, args []string) []string {
	var flags, pos []string
	for i := 0; i < len(args); i++ {
		a := args[i]
		if runtime.GOOS == "windows" && len(a) > 1 && a[0] == '/' {
			a = "-" + a[1:]
		}
		if a == "--" {
			pos = append(pos, args[i+1:]...)
			break
		}
		if len(a) < 2 || a[0] != '-' {
			pos = append(pos, a)
			continue
		}
		flags = append(flags, a)
		name := strings.TrimLeft(a, "-")
		if strings.Contains(name, "=") {
			continue
		}
		f := fs.Lookup(name)
		if f == nil {
			continue
		}
		if bf, ok := f.Value.(interface{ IsBoolFlag() bool }); ok && bf.IsBoolFlag() {
			continue
		}
		if i+1 < len(args) {
			flags = append(flags, args[i+1])
			i++
		}
	}
	return append(flags, pos...)
}

func resolve(ctx context.Context, host string) (net.IP, error) {
	if ip := net.ParseIP(host); ip != nil {
		if ip.To4() == nil {
			return nil, errors.New("IPv6 isn't supported yet, the unicorn only speaks IPv4")
		}
		return ip.To4(), nil
	}
	ips, err := net.DefaultResolver.LookupIP(ctx, "ip4", host)
	if err != nil || len(ips) == 0 {
		return nil, fmt.Errorf("could not find host %s. Please check the name and try again", host)
	}
	return ips[0].To4(), nil
}

func formatRTT(d time.Duration) string {
	if d < time.Millisecond {
		return "time<1ms"
	}
	return fmt.Sprintf("time=%dms", d.Milliseconds())
}

func pongLine(r reply, offset int) string {
	ttl := ""
	if r.ttl >= 0 {
		ttl = fmt.Sprintf(" TTL=%d", r.ttl)
	}
	return fmt.Sprintf("%s %s %s%s %s",
		"✨",
		bold(rainbow("pong!", offset)),
		c(pink, "from "),
		c(sky, r.from.String()+":"),
		c(lilac, fmt.Sprintf("bytes=%d %s%s", r.bytes, formatRTT(r.rtt), ttl)),
	)
}

var sadPhrases = []string{
	"the unicorn wandered off...",
	"lost somewhere over the rainbow...",
	"the pong fell into a cloud...",
	"no sparkle came back...",
}

func pingHost(ctx context.Context, p *pinger, host string, o options, snd *soundPlayer) int {
	ip, err := resolve(ctx, host)
	if err != nil {
		fmt.Println(c(red, "💔 "+err.Error()))
		return 1
	}
	label := ip.String()
	if host != label {
		label = fmt.Sprintf("%s [%s]", host, ip)
	}
	fmt.Printf("\n%s %s %s %s\n\n",
		"🦄", c(pink, "Pinging"), c(sky, label), c(pink, fmt.Sprintf("with %d bytes of sparkle:", o.size)))

	var sent, recv int
	var minRTT, maxRTT, sumRTT time.Duration

loop:
	for seq := 0; o.forever || seq < o.count; seq++ {
		start := time.Now()
		r, err := p.Ping(ip, o.size, o.timeout, seq)
		sent++
		switch {
		case err == errTimeout:
			fmt.Println("💤 " + c(grey, "Request timed out - "+sadPhrases[seq%len(sadPhrases)]))
		case err != nil:
			fmt.Println("💔 " + c(red, err.Error()))
		case r.unreachable != "":
			fmt.Printf("🚧 %s %s\n", c(gold, "neigh! from "+r.from.String()+":"), c(red, r.unreachable))
		default:
			recv++
			if recv == 1 || r.rtt < minRTT {
				minRTT = r.rtt
			}
			if r.rtt > maxRTT {
				maxRTT = r.rtt
			}
			sumRTT += r.rtt
			fmt.Println(pongLine(r, seq))
			snd.Play()
		}

		if !o.forever && seq+1 >= o.count {
			break
		}
		wait := o.interval - time.Since(start)
		if wait < 0 {
			wait = 0
		}
		select {
		case <-ctx.Done():
			break loop
		case <-time.After(wait):
		}
	}
	if ctx.Err() != nil {
		fmt.Println(c(grey, "\n🛑 Ctrl+C - the unicorn takes a break"))
	}

	lost := sent - recv
	loss := 0
	if sent > 0 {
		loss = lost * 100 / sent
	}
	fmt.Printf("\n🌈 %s\n", rainbow("Pong statistics for "+ip.String()+":", 0))
	fmt.Printf("    %s\n", c(lilac, fmt.Sprintf("Packets: Sent = %d, Received = %d, Lost = %d (%d%% loss),", sent, recv, lost, loss)))
	if recv > 0 {
		fmt.Println(c(pink, "Approximate round trip times in milli-seconds:"))
		fmt.Printf("    %s\n", c(lilac, fmt.Sprintf("Minimum = %dms, Maximum = %dms, Average = %dms",
			minRTT.Milliseconds(), maxRTT.Milliseconds(), (sumRTT/time.Duration(recv)).Milliseconds())))
	}
	switch {
	case recv == 0:
		fmt.Println(c(red, "\n💔 No pongs came back... the unicorn is sad."))
		return 1
	case lost > 0:
		fmt.Println(c(gold, "\n🦄 Some pongs got lost in the clouds."))
	default:
		fmt.Println(c(mint, "\n🦄 All pongs returned. The unicorn is pleased! ✨"))
	}
	return 0
}

// ---------- subnet sweep ----------

type hostResult struct {
	ip    net.IP
	sent  int
	recv  int
	first reply
}

type sweepUI struct {
	mu          sync.Mutex
	done, total int
	progress    bool
}

func (u *sweepUI) println(s string) {
	u.mu.Lock()
	defer u.mu.Unlock()
	if u.progress {
		fmt.Print("\r\033[K")
	}
	fmt.Println(s)
	u.drawLocked()
}

func (u *sweepUI) hostDone() {
	u.mu.Lock()
	defer u.mu.Unlock()
	u.done++
	u.drawLocked()
}

func (u *sweepUI) drawLocked() {
	if !u.progress {
		return
	}
	pct := u.done * 100 / u.total
	bar := 20 * u.done / u.total
	fmt.Printf("\r\033[K🦄 %s%s %s",
		rainbow(strings.Repeat("=", bar), 0),
		c(grey, strings.Repeat("·", 20-bar)),
		c(grey, fmt.Sprintf("%d/%d hosts (%d%%)", u.done, u.total, pct)))
}

func (u *sweepUI) clear() {
	if u.progress {
		fmt.Print("\r\033[K")
	}
}

func subnetHosts(cidr string) ([]net.IP, *net.IPNet, error) {
	ip, ipnet, err := net.ParseCIDR(cidr)
	if err != nil {
		return nil, nil, fmt.Errorf("%q is not a valid subnet (try something like 192.168.1.0/24)", cidr)
	}
	if ip.To4() == nil {
		return nil, nil, errors.New("only IPv4 subnets are supported")
	}
	ones, _ := ipnet.Mask.Size()
	if ones < 16 {
		return nil, nil, errors.New("that subnet is too big for one unicorn - use /16 or smaller")
	}
	start := binary.BigEndian.Uint32(ipnet.IP.To4())
	size := uint32(1) << (32 - ones)
	first, last := start, start+size-1
	if size > 2 { // skip network and broadcast addresses
		first++
		last--
	}
	hosts := make([]net.IP, 0, last-first+1)
	for a := first; a <= last; a++ {
		h := make(net.IP, 4)
		binary.BigEndian.PutUint32(h, a)
		hosts = append(hosts, h)
	}
	return hosts, ipnet, nil
}

func sweep(ctx context.Context, p *pinger, cidr string, o options, snd *soundPlayer) int {
	hosts, ipnet, err := subnetHosts(cidr)
	if err != nil {
		fmt.Println(c(red, "💔 "+err.Error()))
		return 1
	}
	count := o.count
	if !o.countSet {
		count = 1
	}
	if !o.timeoutSet {
		o.timeout = time.Second
	}
	workers := min(o.workers, len(hosts))

	fmt.Printf("\n%s %s %s %s\n\n", "🌈", c(pink, "Sweeping"), c(sky, ipnet.String()),
		c(pink, fmt.Sprintf("- %d hosts, %d ping(s) each, %d unicorns galloping in parallel", len(hosts), count, workers)))

	ui := &sweepUI{total: len(hosts), progress: useColor}
	jobs := make(chan net.IP)
	results := make(chan hostResult, len(hosts))
	var wg sync.WaitGroup
	for w := 0; w < workers; w++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for ip := range jobs {
				hr := hostResult{ip: ip}
				for i := 0; i < count && ctx.Err() == nil; i++ {
					r, err := p.Ping(ip, o.size, o.timeout, i)
					hr.sent++
					if err != nil || r.unreachable != "" {
						continue
					}
					hr.recv++
					if hr.recv == 1 {
						hr.first = r
					}
					ui.println(pongLine(r, int(ip[3])))
					snd.Play()
				}
				ui.hostDone()
				results <- hr
			}
		}()
	}

	started := time.Now()
feed:
	for _, h := range hosts {
		select {
		case <-ctx.Done():
			break feed
		case jobs <- h:
		}
	}
	close(jobs)
	wg.Wait()
	close(results)
	ui.clear()

	var all []hostResult
	for hr := range results {
		all = append(all, hr)
	}
	sort.Slice(all, func(i, j int) bool { return bytes.Compare(all[i].ip, all[j].ip) < 0 })

	if ctx.Err() != nil {
		fmt.Println(c(grey, "\n🛑 Ctrl+C - sweep interrupted, here's what we found so far"))
	}

	alive := 0
	fmt.Printf("\n🌈 %s\n", rainbow("Sweep results for "+ipnet.String()+":", 0))
	for _, hr := range all {
		if hr.recv > 0 {
			alive++
			ttl := ""
			if hr.first.ttl >= 0 {
				ttl = fmt.Sprintf("  TTL=%d", hr.first.ttl)
			}
			fmt.Printf("    🦄 %-15s %s\n", c(sky, hr.ip.String()),
				c(lilac, fmt.Sprintf("%d/%d pongs  %s%s", hr.recv, hr.sent, formatRTT(hr.first.rtt), ttl)))
		} else if o.showAll {
			fmt.Printf("    💤 %-15s %s\n", c(grey, hr.ip.String()), c(grey, "silent"))
		}
	}
	elapsed := time.Since(started).Round(100 * time.Millisecond)
	if alive == 0 {
		fmt.Println(c(red, fmt.Sprintf("\n💔 Not a single pong from %d hosts (%s). The meadow is empty.", len(all), elapsed)))
		return 1
	}
	fmt.Println(c(mint, fmt.Sprintf("\n✨ %d of %d hosts said pong! (%s)", alive, len(all), elapsed)))
	return 0
}
