package main

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	md "github.com/JohannesKaufmann/html-to-markdown"
	"github.com/gen2brain/go-fitz"
	// "github.com/ledongthuc/pdf"
	// "github.com/sukeesh/markitdown-go/pkg/pdfconverter"
)

func main() {
	fmt.Println("Hello, World!")

	inputDir, _ := filepath.Abs("./inputs/")
	items, _ := os.ReadDir(inputDir)
	for _, item := range items {
		if !item.IsDir() {
			fmt.Printf("%+v", item)

			outputMarkdownFilePath := filepath.Join("outputs", item.Name())
			inputFile := filepath.Join(inputDir, item.Name())
			output, err := extractText(inputFile)

			if err != nil {
				fmt.Println(err)
			}

			file, err := os.Create(outputMarkdownFilePath)
			if err != nil {
				fmt.Println(err)
			}
			defer file.Close()

			// _, err = io.Copy(file, output)
			_, err = file.Write([]byte(output.String()))
			// err = os.WriteFile(outputMarkdownFilePath, output, 0644)
			if err != nil {
				fmt.Println("failed to write markdown file: %v", err)
			}

			// err := pdfconverter.ConvertPDFToMarkdown(filepath.Join(inputDir, item.Name()), outputMarkdownFilePath, "inputs/assets")

			if err != nil {
				fmt.Println(err)
			}
			// fmt.Println(outputMarkdownFilePath)
		}
	}
}

func extractText(pdfPath string) (strings.Builder, error) {
	doc, err := fitz.New(pdfPath)
	var sb strings.Builder

	if err != nil {
		return sb, err
	}

	defer doc.Close()

	// Extract pages as images
	for n := 0; n < doc.NumPage(); n++ {
		stuff, err := doc.Text(n)
		// stuff, err := doc.HTML(n, false)

		if err != nil {
			return sb, err
		}

		_, err = sb.WriteString(stuff)

		if err != nil {
			return sb, err
		}
	}

	return sb, nil
}

// extractText extracts plain text content from a PDF file using the ledongthuc/pdf library.
// It preserves line breaks and returns the text as a single string.
// func extractText(pdfPath string) (string, error) {
// 	f, r, err := pdf.Open(pdfPath)
// 	if err != nil {
// 		return "", err
// 	}
// 	defer f.Close()
//
// 	var buf bytes.Buffer
// 	sentences, err := r.GetStyledTexts()
// 	if err != nil {
// 		return "", err
// 	}
//
// 	for _, line := range sentences {
// 		line.Font
// 	}
// 	scanner := bufio.NewScanner(b)
// 	for scanner.Scan() {
// 		line := scanner.Text()
// 		buf.WriteString(line + "\n")
// 	}
//
// 	if err := scanner.Err(); err != nil {
// 		return "", err
// 	}
//
// 	return buf.String(), nil
// }
