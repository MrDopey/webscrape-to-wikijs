package main

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/sukeesh/markitdown-go/pkg/pdfconverter"
)

func main() {
	fmt.Println("Hello, World!")

	curr, _ := filepath.Abs("./inputs/")
	items, _ := os.ReadDir(curr)
	for _, item := range items {
		if !item.IsDir() {
			fmt.Printf("%+v", item)

			outputMarkdownString := filepath.Join("outputs", item.Name())
			err := pdfconverter.ConvertPDFToMarkdown(filepath.Join(curr, item.Name()), outputMarkdownString, "inputs/assets")

			if err != nil {
				fmt.Println(err)
			}
			fmt.Println(outputMarkdownString)
		}
	}
}
