default __fragment_parameters_metadata := {}
__fragment_parameters_metadata := data[input.namespace].parameters_api {
    data[input.namespace].parameters_api
}
parameter(name) := data.framework.extract_parameter(name, __fragment_parameters, __fragment_parameters_metadata)
