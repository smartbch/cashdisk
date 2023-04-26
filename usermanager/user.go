package usermanager

import "github.com/dgraph-io/badger/v3"

type User struct {
	Name string
	Pass string

	DB *badger.DB
}
