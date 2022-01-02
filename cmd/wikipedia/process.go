package main

import (
	"fmt"

	"gitlab.com/tozd/go/errors"
)

func processArticle(article Article) errors.E {
	fmt.Printf("%+v\n", article)
	return nil
}
