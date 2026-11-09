// Command site renders Kando's landing page, kando.yldm.ai, into static files: one page per interface language, with the screenshots, site logos and icons it shows. Run it from the repository root:
//
//	go run ./apps/site -out apps/site/dist
package main

import (
	"flag"
	"fmt"
	"os"
	"strings"
)

func main() {
	root := flag.String("root", ".", "repository root, where apps/site, docs/assets and apps/kando are")
	out := flag.String("out", "apps/site/dist", "directory to write the site to; it is emptied first")
	flag.Parse()
	rep, err := build(*root, *out)
	if err != nil {
		fmt.Fprintln(os.Stderr, "site:", err)
		os.Exit(1)
	}
	codes := make([]string, len(rep.Langs))
	for i, l := range rep.Langs {
		codes[i] = l.Code
	}
	fmt.Printf("site: rendered %s into %s\n", strings.Join(codes, ", "), *out)
}
