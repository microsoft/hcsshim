package hcsshim

import "github.com/Microsoft/hcsshim/hns"

// HotAttachEndpoint makes a HCS Call to attach the endpoint to the container
func HotAttachEndpoint(containerID string, endpointID string) error {
	endpoint, err := hns.GetHNSEndpointByID(endpointID)
	if err != nil {
		return err
	}
	isAttached, err := endpoint.IsAttached(containerID)
	if isAttached {
		return err
	}
	return modifyNetworkEndpoint(containerID, endpointID, Add)
}

// HotDetachEndpoint makes a HCS Call to detach the endpoint from the container
func HotDetachEndpoint(containerID string, endpointID string) error {
	endpoint, err := hns.GetHNSEndpointByID(endpointID)
	if err != nil {
		return err
	}
	isAttached, err := endpoint.IsAttached(containerID)
	if !isAttached {
		return err
	}
	return modifyNetworkEndpoint(containerID, endpointID, Remove)
}

// ModifyContainer corresponding to the container id, by sending a request
func modifyContainer(id string, request *ResourceModificationRequestResponse) error {
	container, err := OpenContainer(id)
	if err != nil {
		if IsNotExist(err) {
			return ErrComputeSystemDoesNotExist
		}
		return getInnerError(err)
	}
	defer container.Close()
	err = container.Modify(request)
	if err != nil {
		if IsNotSupported(err) {
			return ErrPlatformNotSupported
		}
		return getInnerError(err)
	}

	return nil
}

func modifyNetworkEndpoint(containerID string, endpointID string, request RequestType) error {
	requestMessage := &ResourceModificationRequestResponse{
		Resource: Network,
		Request:  request,
		Data:     endpointID,
	}
	err := modifyContainer(containerID, requestMessage)

	if err != nil {
		return err
	}

	return nil
}

