package main

type reader interface {
	Read() (record []string, err error)
}
