// This example writes a generic archive through the shared streaming sink.
package main

import (
	"flag"
	"fmt"
	"io"
	"os"

	"github.com/marcuoli/go-xlsx-report/zipio"
)

func main() {
	output := flag.String("output", "", "explicit output file outside source")
	flag.Parse()
	if *output == "" {
		fmt.Fprintln(os.Stderr, "--output is required")
		os.Exit(2)
	}
	if err := write(*output); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}
func write(path string) (returnedErr error) {
	file, err := os.OpenFile(path, os.O_CREATE|os.O_EXCL|os.O_RDWR, 0600)
	if err != nil {
		return err
	}
	defer file.Close()
	writer, err := zipio.NewWriter(file)
	if err != nil {
		return err
	}
	part, err := writer.Create("example.txt")
	if err != nil {
		return err
	}
	if _, err := io.WriteString(part, "exact value: 9007199254740993\n"); err != nil {
		return err
	}
	if err := writer.Close(); err != nil {
		return err
	}
	return file.Sync()
}
