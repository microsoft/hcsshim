package regopolicyinterpreter

import (
	"bytes"
	"context"
	_ "embed"
	"encoding/json"
	"fmt"
	"sync"

	"github.com/open-policy-agent/opa/ast"
	"github.com/open-policy-agent/opa/rego"
	"github.com/open-policy-agent/opa/storage/inmem"
	"github.com/open-policy-agent/opa/topdown"
	"github.com/pkg/errors"
)

type LogLevel int32

const (
	LogNone LogLevel = iota
	// LogInfo logs the output of Rego print() statements in the policy
	LogInfo
	// LogResults logs the result objects returned from each query
	LogResults
	// LogMetadata logs the full metadata state after each query
	LogMetadata
)

type RegoPolicyInterpreter struct {
	// Mutex to ensure query objects cannot change during query execution
	dataAndModulesMutex sync.Mutex
	// Rego which describes policy behavior (see above)
	code string
	// Rego data namespace
	data map[string]interface{}
	// Modules
	modules map[string]*RegoModule
	// Compiled modules
	compiledModules *ast.Compiler

	logger InterpreterLogger
}

type InterpreterOpt func(rpi *RegoPolicyInterpreter) error

func WithLogger(logger InterpreterLogger) InterpreterOpt {
	return func(rpi *RegoPolicyInterpreter) error {
		rpi.logger = logger
		return nil
	}
}

type RegoModule struct {
	// The Rego namespace of the module
	Namespace string
	// The feed from which the module was obtained
	Feed string
	// The issuer of the module
	Issuer string
	// The module Rego code
	Code string
}

/* See README for more details on Metadata */

type regoMetadata map[string]map[string]interface{}

type regoMetadataAction string

const (
	metadataAdd    regoMetadataAction = "add"
	metadataUpdate regoMetadataAction = "update"
	metadataRemove regoMetadataAction = "remove"
)

type regoMetadataOperation struct {
	Action regoMetadataAction `json:"action"`
	Name   string             `json:"name"`
	Key    string             `json:"key"`
	Value  interface{}        `json:"value"`
}

// RegoQueryResult is the result from a policy query
type RegoQueryResult map[string]interface{}

// deep copy for an object
func copyObject(data map[string]interface{}) (map[string]interface{}, error) {
	objJSON, err := json.Marshal(data)
	if err != nil {
		return nil, err
	}

	var objCopy map[string]interface{}
	err = json.Unmarshal(objJSON, &objCopy)
	if err != nil {
		return nil, err
	}

	return objCopy, nil
}

// deep copy for a value
func copyValue(value interface{}) (interface{}, error) {
	valueJSON, err := json.Marshal(value)
	if err != nil {
		return nil, err
	}

	var valueCopy interface{}
	err = json.Unmarshal(valueJSON, &valueCopy)
	if err != nil {
		return nil, err
	}

	return valueCopy, nil
}

// NewRegoPolicyInterpreter creates a new RegoPolicyInterpreter, using the code provided.
// inputData is the Rego data which should be used as the initial state
// of the interpreter. A deep copy is performed on it such that it will
// not be modified.
func NewRegoPolicyInterpreter(code string, inputData map[string]interface{}, opts ...InterpreterOpt) (*RegoPolicyInterpreter, error) {
	data, err := copyObject(inputData)
	if err != nil {
		return nil, fmt.Errorf("unable to copy the input data: %w", err)
	}

	if _, ok := data["metadata"]; !ok {
		data["metadata"] = make(regoMetadata)
	}

	policy := &RegoPolicyInterpreter{
		code:    code,
		data:    data,
		modules: make(map[string]*RegoModule),
	}

	for _, opt := range opts {
		if err := opt(policy); err != nil {
			return nil, err
		}
	}

	return policy, nil
}

// AddModule adds the specified module to the interpreter such that it will be
// loaded along with the policy during query execution. The provided id
// should be used to refer to it for other methods. This will also
// invalidate the compliation artifact (i.e. Compile must be called again)
func (r *RegoPolicyInterpreter) AddModule(id string, module *RegoModule) {
	r.dataAndModulesMutex.Lock()
	defer r.dataAndModulesMutex.Unlock()
	r.modules[id] = module
	r.compiledModules = nil
}

// RemoveModule removes the specified module such that it will no longer be loaded.
// This will also invalidate the compliation artifact (i.e. Compile must be
// called again)
func (r *RegoPolicyInterpreter) RemoveModule(id string) {
	r.dataAndModulesMutex.Lock()
	defer r.dataAndModulesMutex.Unlock()
	delete(r.modules, id)
	r.compiledModules = nil
}

// IsModuleActive returns whether the specified module is currently active, i.e. being loaded
// along with the policy.
func (r *RegoPolicyInterpreter) IsModuleActive(id string) bool {
	r.dataAndModulesMutex.Lock()
	defer r.dataAndModulesMutex.Unlock()
	_, ok := r.modules[id]
	return ok
}

// GetData attempts to retrieve and return a copy of the data value with the
// specified key.
func (r *RegoPolicyInterpreter) GetData(key string) (interface{}, error) {
	r.dataAndModulesMutex.Lock()
	defer r.dataAndModulesMutex.Unlock()
	if value, ok := r.data[key]; ok {
		cp, err := copyValue(value)
		if err != nil {
			return nil, fmt.Errorf("unable to copy value: %w", err)
		}
		return cp, nil
	}

	return nil, fmt.Errorf("data value not found for `%s`", key)
}

// UpdateData will perform an update to a value which is already within the data
// A deep copy will be performed on the value.
func (r *RegoPolicyInterpreter) UpdateData(key string, value interface{}) error {
	r.dataAndModulesMutex.Lock()
	defer r.dataAndModulesMutex.Unlock()
	value, err := copyValue(value)
	if err != nil {
		return fmt.Errorf("unable to copy value: %w", err)
	}

	if _, ok := r.data[key]; ok {
		r.data[key] = value
		return nil
	} else {
		return fmt.Errorf("data value not found for `%s`", key)
	}
}

// GetMetadata retrieves a copy of a single metadata item from the policy.
func (r *RegoPolicyInterpreter) GetMetadata(name string, key string) (interface{}, error) {
	r.dataAndModulesMutex.Lock()
	defer r.dataAndModulesMutex.Unlock()

	metadataRoot, ok := r.data["metadata"].(regoMetadata)
	if !ok {
		return nil, errors.New("illegal interpreter state: invalid metadata object type")
	}

	if metadata, ok := metadataRoot[name]; ok {
		if value, ok := metadata[key]; ok {
			value, err := copyValue(value) //nolint:govet // shadow
			if err != nil {
				return nil, fmt.Errorf("unable to copy value: %w", err)
			}

			return value, nil
		} else {
			return nil, fmt.Errorf("value not found in %s for key %s", name, key)
		}
	} else {
		return nil, fmt.Errorf("metadata not found for name %s", name)
	}
}

func newRegoMetadataOperation(operation interface{}) (*regoMetadataOperation, error) {
	var metadataOp regoMetadataOperation

	data, ok := operation.(map[string]interface{})
	if !ok {
		return nil, errors.New("unable to load metadata object")
	}
	metadataOp.Name, ok = data["name"].(string)
	if !ok {
		return nil, errors.New("unable to load metadata name")
	}
	action, ok := data["action"].(string)
	if !ok {
		return nil, errors.New("unable to load metadata action")
	}
	metadataOp.Action = regoMetadataAction(action)
	metadataOp.Key, ok = data["key"].(string)
	if !ok {
		return nil, errors.New("unable to load metadata key")
	}

	if metadataOp.Action != metadataRemove {
		metadataOp.Value, ok = data["value"]
		if !ok {
			return nil, errors.New("unable to load metadata value")
		}
	}

	return &metadataOp, nil
}

func (m regoMetadata) getOrCreate(name string) map[string]interface{} {
	if metadata, ok := m[name]; ok {
		return metadata
	}

	metadata := make(map[string]interface{})
	m[name] = metadata
	return metadata
}

// dataAndModulesMutex must be held before calling this
func (r *RegoPolicyInterpreter) updateMetadata(ops []*regoMetadataOperation) error {
	metadataRoot, ok := r.data["metadata"].(regoMetadata)
	if !ok {
		return errors.New("illegal interpreter state: invalid metadata object type")
	}

	for _, op := range ops {
		metadata := metadataRoot.getOrCreate(op.Name)
		switch op.Action {
		case metadataAdd:
			if _, ok := metadata[op.Key]; ok {
				return fmt.Errorf("cannot add metadata value, key %s[%s] already exists", op.Name, op.Key)
			} else {
				metadata[op.Key] = op.Value
			}
		case metadataUpdate:
			metadata[op.Key] = op.Value
		case metadataRemove:
			delete(metadata, op.Key)
		default:
			return fmt.Errorf("unrecognized metadata action: %s", op.Action)
		}
	}

	r.logMetadata()

	return nil
}

// EnableLogging enables logging to the provided path at the specified level.
func (r *RegoPolicyInterpreter) EnableLogging(path string, level LogLevel) error {
	// this mutex ensures no-one reads compiledModules before we clear it
	r.dataAndModulesMutex.Lock()
	defer r.dataAndModulesMutex.Unlock()

	r.compiledModules = nil
	filelogger, err := NewFileLogger(path, level)
	if err != nil {
		return err
	}
	r.logger = filelogger
	r.logInfo(context.TODO(), "Logging Enabled at level %d", level)

	return nil
}

// SetLogLevel sets the logging level. To actually produce a log, however, EnableLogging
// must be called first.
func (r *RegoPolicyInterpreter) SetLogLevel(level LogLevel) {
	// this mutex ensures no-one reads compiledModules before we clear it
	r.dataAndModulesMutex.Lock()
	defer r.dataAndModulesMutex.Unlock()

	r.compiledModules = nil
	if r.logger != nil {
		r.logger.SetLevel(level)
	}
}

// DisableLogging disables logging and closes the underlying log file.
func (r *RegoPolicyInterpreter) DisableLogging() error {
	// this mutex ensures no-one reads compiledModules before we clear it
	r.dataAndModulesMutex.Lock()
	defer r.dataAndModulesMutex.Unlock()

	r.compiledModules = nil
	if r.logger == nil {
		return nil
	}
	return r.logger.Close(context.TODO())
}

// dataAndModulesMutex must be held before calling this
func (r *RegoPolicyInterpreter) compile() error {
	modules := make(map[string]string)
	for _, module := range r.modules {
		modules[module.Namespace+".rego"] = module.Code
	}

	modules["policy.rego"] = r.code

	printStatements := false
	if r.logger != nil && r.logger.Level() != LogNone {
		printStatements = true
	}
	options := ast.CompileOpts{
		EnablePrintStatements: printStatements,
	}

	if compiled, err := ast.CompileModulesWithOpt(modules, options); err == nil {
		r.compiledModules = compiled
		return nil
	} else {
		return fmt.Errorf("rego compilation failed: %w", err)
	}
}

// Compile compiles the policy and its modules. This will increase the speed of policy
// execution.
func (r *RegoPolicyInterpreter) Compile() error {
	r.dataAndModulesMutex.Lock()
	defer r.dataAndModulesMutex.Unlock()

	return r.compile()
}

func (r *RegoPolicyInterpreter) logInfo(ctx context.Context, message string, args ...interface{}) {
	if r.logger == nil {
		return
	}
	r.logger.LogInfo(ctx, message, args...)
}

func (r *RegoPolicyInterpreter) logResult(ctx context.Context, rule string, resultSet interface{}) {
	if r.logger == nil {
		return
	}
	r.logger.LogResult(ctx, rule, resultSet)
}

func (r *RegoPolicyInterpreter) logMetadata() {
	if r.logger == nil {
		return
	}
	r.logger.LogMetadata(context.TODO(), r.data)
}

// Value returns the raw value from a Rego query result.
func (r RegoQueryResult) Value(key string) (interface{}, error) {
	if value, ok := r[key]; ok {
		return value, nil
	} else {
		return nil, fmt.Errorf("unable to find value for key '%s'", key)
	}
}

// Object attempts to interpret the result value as an object
func (r RegoQueryResult) Object(key string) (map[string]interface{}, error) {
	if value, ok := r[key]; ok {
		if obj, ok := value.(map[string]interface{}); ok {
			return obj, nil
		} else {
			return nil, fmt.Errorf("value for '%s' is not an object", key)
		}
	} else {
		return nil, fmt.Errorf("unable to find value for key '%s'", key)
	}
}

// Bool attempts to interpret a result value as a boolean.
func (r RegoQueryResult) Bool(key string) (bool, error) {
	if value, ok := r[key]; ok {
		if flag, ok := value.(bool); ok {
			return flag, nil
		} else {
			return false, fmt.Errorf("value for '%s' is not a boolean", key)
		}
	} else {
		return false, fmt.Errorf("unable to find value for key '%s'", key)
	}
}

// String attempts to interpret the result value as a string.
func (r RegoQueryResult) String(key string) (string, error) {
	if value, ok := r[key]; ok {
		if str, ok := value.(string); ok {
			return str, nil
		} else {
			return "", fmt.Errorf("value for '%s' is not a string", key)
		}
	} else {
		return "", fmt.Errorf("unable to find value for key '%s'", key)
	}
}

// Float attempts to interpret the result value as a floating point number.
func (r RegoQueryResult) Float(key string) (float64, error) {
	if value, ok := r[key]; ok {
		if number, ok := value.(json.Number); ok {
			if floatValue, err := number.Float64(); err == nil {
				return floatValue, nil
			} else {
				return 0, err
			}
		} else {
			return 0, fmt.Errorf("value for %s is not a json Number", key)
		}
	} else {
		return 0, fmt.Errorf("unable to find value for key '%s'", key)
	}
}

// Int attempts to interpret the result value as an integer.
func (r RegoQueryResult) Int(key string) (int, error) {
	if value, ok := r[key]; ok {
		if number, ok := value.(json.Number); ok {
			if intValue, err := number.Int64(); err == nil {
				return int(intValue), nil
			} else {
				return 0, err
			}
		} else {
			return 0, errors.New("value is not a json Number")
		}
	} else {
		return 0, fmt.Errorf("unable to find value for key '%s'", key)
	}
}

// IsEmpty tests if the query result is empty.
func (r RegoQueryResult) IsEmpty() bool {
	return len(r) == 0
}

// Union creates a new result object which is the union of this result with
// another result, in which the results of the other will take precedence.
func (r RegoQueryResult) Union(other RegoQueryResult) RegoQueryResult {
	result := make(RegoQueryResult)
	for key := range r {
		result[key] = r[key]
	}
	for key := range other {
		result[key] = other[key]
	}
	return result
}

// dataAndModulesMutex must be held before calling this
func (r *RegoPolicyInterpreter) query(ctx context.Context, rule string, input map[string]interface{}) (rego.ResultSet, error) {
	store := inmem.NewFromObject(r.data)

	var buf bytes.Buffer
	printStatements := false
	if r.logger != nil && r.logger.Level() != LogNone {
		printStatements = true
	}
	query := rego.New(
		rego.Query(rule),
		rego.Input(input),
		rego.Store(store),
		rego.EnablePrintStatements(printStatements),
		rego.PrintHook(topdown.NewPrintHook(&buf)),
		rego.Compiler(r.compiledModules))

	resultSet, err := query.Eval(ctx)
	output := buf.String()

	r.logInfo(ctx, output)

	return resultSet, err
}

func (r *RegoPolicyInterpreter) RawQuery(ctx context.Context, rule string, input map[string]interface{}) (rego.ResultSet, error) {
	// this mutex ensures no other threads modify the data and compiledModules fields during query execution
	r.dataAndModulesMutex.Lock()
	defer r.dataAndModulesMutex.Unlock()

	if r.compiledModules == nil {
		err := r.compile()
		if err != nil {
			return nil, fmt.Errorf("error when compiling modules: %w", err)
		}
	}

	resultSet, err := r.query(ctx, rule, input)
	if err != nil {
		return nil, err
	}

	r.logResult(ctx, rule, resultSet)

	return resultSet, nil
}

// Query queries the policy with the given rule and input data and returns the result.
func (r *RegoPolicyInterpreter) Query(ctx context.Context, rule string, input map[string]interface{}) (RegoQueryResult, error) {
	// this mutex ensures no other threads modify the data and compiledModules fields during query execution
	r.dataAndModulesMutex.Lock()
	defer r.dataAndModulesMutex.Unlock()

	if r.compiledModules == nil {
		err := r.compile()
		if err != nil {
			return nil, fmt.Errorf("error when compiling modules: %w", err)
		}
	}

	rawResult, err := r.query(ctx, rule, input)
	if err != nil {
		return nil, err
	}

	result := make(RegoQueryResult)
	if len(rawResult) == 0 {
		return result, nil
	}

	resultSet, ok := rawResult[0].Expressions[0].Value.(map[string]interface{})
	if !ok {
		return nil, errors.New("unable to load results object from Rego query")
	}

	r.logResult(ctx, rule, resultSet)

	var ops []*regoMetadataOperation
	if rawMetadata, ok := resultSet["metadata"]; ok {
		metadata, ok := rawMetadata.([]interface{})
		if !ok {
			return nil, errors.New("error loading metadata array: invalid type")
		}

		for _, value := range metadata {
			op, err := newRegoMetadataOperation(value)
			if err != nil {
				return nil, fmt.Errorf("error loading metadata operation: %w", err)
			}
			ops = append(ops, op)
		}

		if len(ops) > 0 {
			err = r.updateMetadata(ops)
			if err != nil {
				return nil, fmt.Errorf("error applying metadata operations: %w", err)
			}
		}
	}

	for name, value := range resultSet {
		if name == "metadata" {
			continue
		} else {
			result[name] = value
		}
	}

	return result, nil
}

// ModuleID computes a unique ID for a Module from its issuer and feed.
func ModuleID(issuer string, feed string) string {
	return issuer + ">" + feed
}

// ID is the unique ID of a module.
func (f RegoModule) ID() string {
	return ModuleID(f.Issuer, f.Feed)
}
