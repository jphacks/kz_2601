// Package dsl holds the Server-Driven UI schema shared by all services.
// NOTE: placeholder. The real schema is defined after the Day1 design sync.
package dsl

// App is the root document that describes a whole generated application.
type App struct {
	ID      string `json:"id"`
	Version int    `json:"version"`
	Name    string `json:"name"`
}
