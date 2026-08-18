package module

subtract := {"result": result} if {
    result := input.a - input.b
}

subtract := {"result": result} if {
    result := concat("-", [input.a, input.b])
}