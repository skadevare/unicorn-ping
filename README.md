# 🦄 unicornping

Works like `ping`, but every reply is a **pong!** It's rainbow colored, plays a "pong" sound for each reply, and can sweep a whole subnet.
![Unicorn ping](https://github.com/skadevare/unicorn-ping/blob/main/example-unicornping.png)

```
✨ pong! from 8.8.8.8: bytes=32 time=22ms TTL=113
```

## Usage

```
unicornping [options] <host | ip | subnet/cidr>
```

| Option | Meaning |
|---|---|
| `-n N` / `-c N` | number of pings (default 4; 1 per host in subnet mode) |
| `-t` | ping until Ctrl+C |
| `-w MS` | timeout per ping (default 4000; 1000 in subnet mode) |
| `-i MS` | delay between pings (default 1000) |
| `-l BYTES` | payload size (default 32) |
| `-mute` / `-nosound` | no pong sound |
| `-nocolor` | plain output, no rainbows |
| `-nobanner` | skip the unicorn |
| `-workers N` | subnet mode: parallel pings (default 128) |
| `-all` | subnet mode: also list silent hosts |

Options can go before or after the target. On Windows, `/t`-style switches work too.
To mute the sound permanently, set the environment variable `UNICORNPING_MUTE=1`.

Examples:

```
unicornping google.com -t
unicornping -mute 1.1.1.1 -n 10
unicornping 192.168.1.0/24
unicornping 10.0.0.0/22 -all -mute
```

## Building

Requires Go 1.22 or newer. Run `.\build.ps1` to produce binaries in `dist\` for Windows (amd64/arm64) and macOS (Intel/Apple Silicon).
They're cross-compiled with no cgo, so you can build the Mac versions from Windows.

On macOS, the first run of a downloaded binary may be blocked by Gatekeeper. Clear it with:

```
xattr -d com.apple.quarantine ./unicornping-mac-arm64
chmod +x ./unicornping-mac-arm64
```

## Notes

- **Windows** uses the same `IcmpSendEcho` API as `ping.exe`, so it needs no admin rights. Sound uses the built-in `PlaySound`.
- **macOS** uses unprivileged ICMP sockets, so no `sudo` is needed. Sound plays through `afplay`.
- The pong sound is synthesized in code, so the binary ships no audio files.
- Only IPv4 is supported. Subnets can be up to /16.
