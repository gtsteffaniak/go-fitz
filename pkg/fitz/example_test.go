package fitz_test

import (
	"fmt"

	"github.com/gtsteffaniak/go-fitz/pkg/fitz"
)

func ExampleNew() {
	doc, err := fitz.New("testdata/test.pdf")
	if err != nil {
		panic(err)
	}
	defer func() { _ = doc.Close() }()

	fmt.Println(doc.NumPage())
	// Output: 84
}
