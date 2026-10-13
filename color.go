package main

import (
	"fmt"
	"strings"
)

var useColor bool

var rainbowPalette = []int{196, 208, 226, 46, 51, 33, 129, 201}

const (
	pink   = 213
	purple = 141
	lilac  = 183
	mint   = 121
	sky    = 117
	gold   = 221
	grey   = 245
	red    = 204
)

func c(code int, s string) string {
	if !useColor {
		return s
	}
	return fmt.Sprintf("\033[38;5;%dm%s\033[0m", code, s)
}

func bold(s string) string {
	if !useColor {
		return s
	}
	return "\033[1m" + s + "\033[0m"
}

// rainbow colors each visible character with the next rainbow color.
func rainbow(s string, offset int) string {
	if !useColor {
		return s
	}
	var b strings.Builder
	i := offset
	for _, r := range s {
		if r == ' ' {
			b.WriteRune(r)
			continue
		}
		fmt.Fprintf(&b, "\033[38;5;%dm%c", rainbowPalette[i%len(rainbowPalette)], r)
		i++
	}
	b.WriteString("\033[0m")
	return b.String()
}

var unicornArt = []string{
	`                        /`,
	`                       //`,
	`          .~~~~~.     //`,
	`        .~~~~~~~~~. _//_`,
	`       ~~~~~~~  /       \`,
	`      ~~~~~~~  |   o     \__`,
	`      ~~~~~~   |            )`,
	`       ~~~~~    \     .____/`,
	`        ~~~      |    |`,
	`                 |    |`,
}

func printBanner() {
	if !useColor {
		fmt.Println("unicorn-ping " + version + " - pong! for every ping")
		return
	}
	fmt.Println()
	for y, line := range unicornArt {
		var b strings.Builder
		for x, r := range line {
			switch {
			case r == ' ':
				b.WriteRune(r)
			case r == '/' && y < 4 && x > 18:
				b.WriteString(c(gold, string(r))) // the horn
			case r == '~':
				b.WriteString(c(rainbowPalette[(x+y)/2%len(rainbowPalette)], string(r))) // the mane
			case r == 'o':
				b.WriteString(c(purple, string(r)))
			default:
				b.WriteString(c(lilac, string(r)))
			}
		}
		fmt.Println(b.String())
	}
	fmt.Printf("\n   %s  %s\n", rainbow("U N I C O R N   P I N G", 0), c(grey, "v"+version))
	fmt.Printf("   %s\n", c(pink, "✨ pong! for every ping 🌈"))
}
