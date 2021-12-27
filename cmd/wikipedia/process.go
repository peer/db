package main

import (
	"fmt"

	"gitlab.com/tozd/go/errors"
)

type Entity map[string]interface{}

func processEntity(entity Entity) errors.E {
	fmt.Printf("%+v\n", entity)
	return nil
}
