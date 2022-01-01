package main

import (
	"fmt"

	"gitlab.com/tozd/go/errors"
)

func processEntity(entity Entity) errors.E {
	fmt.Printf("%+v\n", entity)
	return nil
}
