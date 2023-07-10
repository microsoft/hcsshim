# tools

Based on [offical guidance][tools], the recommended approach to track tool dependencies is via `tools.go`.
To avoid introducing additional imports and vendoring into cosumers that import hcsshim, a dedicated
module for tools is used until go slims down what is imported/vendored from dependencies.

The `github.com/Microsoft/hcsshim` import ensures that, whenever possible, tools use the same imports as the
main repo (specifically, `grpc` imports).

[tools]: https://github.com/golang/go/wiki/Modules#how-can-i-track-tool-dependencies-for-a-module
