// Command adminpass reads a password from stdin and prints its bcrypt hash to
// stdout. Use the output as the ADMIN_PASSWORD_HASH environment variable.
//
// Usage:
//
//	make admin-password
//
// Or directly:
//
//	go run ./cmd/adminpass
package main

import (
	"bufio"
	"fmt"
	"os"
	"strings"

	"golang.org/x/crypto/bcrypt"
)

const minPasswordLen = 12

func main() {
	fmt.Fprint(os.Stderr, "Enter password: ")

	scanner := bufio.NewScanner(os.Stdin)
	if !scanner.Scan() {
		if err := scanner.Err(); err != nil {
			fmt.Fprintln(os.Stderr, "error reading password:", err)
		} else {
			fmt.Fprintln(os.Stderr, "no password provided")
		}
		os.Exit(1)
	}

	password := strings.TrimRight(scanner.Text(), "\r\n")
	if len(password) < minPasswordLen {
		fmt.Fprintf(os.Stderr, "password must be at least %d characters\n", minPasswordLen)
		os.Exit(1)
	}

	hash, err := bcrypt.GenerateFromPassword([]byte(password), bcrypt.DefaultCost)
	if err != nil {
		fmt.Fprintln(os.Stderr, "error hashing password:", err)
		os.Exit(1)
	}

	fmt.Println(string(hash))
}
