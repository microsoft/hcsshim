default __fragment_parameters_metadata := {}
__fragment_parameters_metadata := data[input.namespace].parameters {
    data[input.namespace].parameters
}
parameter(name) := data.framework.extract_parameter(name, __fragment_parameters, __fragment_parameters_metadata)
