// Command migrations writes the module's next migration
// (kosatka.GenerateMigrations), run from the module's root with the two
// test databases running:
//
//	KOSATKA_TEST_MYSQL_DSN=… KOSATKA_TEST_POSTGRES_DSN=… go run ./cmd/migrations -name <name>
//	… go run ./cmd/migrations -check
package main

import (
	"github.com/kosatka-cms/kosatka"

	_ "github.com/kosatka-cms/kosatka-module-workflow"
)

func main() { kosatka.GenerateMigrations() }
