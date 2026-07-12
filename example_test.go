package glob_test

import (
	"errors"
	"fmt"

	"github.com/greatliontech/glob"
)

func Example() {
	pattern := glob.MustCompile("{cmd,internal}/**/*.{go,mod}")

	for _, path := range []string{
		"cmd/main.go",
		"internal/net/http.go",
		"cmd/main.sum",
		"main.go",
	} {
		fmt.Printf("%s: %t\n", path, pattern.Match(path))
	}

	// Output:
	// cmd/main.go: true
	// internal/net/http.go: true
	// cmd/main.sum: false
	// main.go: false
}

func ExampleWithSeparator() {
	pattern := glob.MustCompile("**.example.com", glob.WithSeparator('.'))

	fmt.Println(pattern.Match("example.com"))
	fmt.Println(pattern.Match("api.eu.example.com"))
	fmt.Println(pattern.Match("example.net"))

	// Output:
	// true
	// true
	// false
}

func ExampleCompile() {
	_, err := glob.Compile("[z-a]")
	var compileErr *glob.CompileError
	if errors.As(err, &compileErr) {
		fmt.Printf("invalid pattern at byte %d\n", compileErr.Offset)
	}

	// Output:
	// invalid pattern at byte 2
}
