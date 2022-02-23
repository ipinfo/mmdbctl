package main

type writer interface {
	Write(record []string) error
	Flush()
	Error() error
}
