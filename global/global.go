// config/config.go
package global

var global_runtime string

// Setter function
func SetGlobalRuntime(value string) {
    global_runtime = value
}

// Getter function
func GetGlobalRuntime() string {
    return global_runtime
}
