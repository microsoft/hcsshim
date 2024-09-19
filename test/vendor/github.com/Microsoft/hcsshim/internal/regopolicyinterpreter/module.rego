package module

subtract := {"result": result} {
    result := input.a - input.b
}

subtract := {"result": result} {
    result := concat("-", [input.a, input.b])
}