package regopolicyinterpreter

import (
	"bytes"
	"context"
	_ "embed"
	"encoding/json"
	"fmt"
	"log"
	"os"
	"sync"

	"github.com/open-policy-agent/opa/ast"
	"github.com/open-policy-agent/opa/rego"
	"github.com/open-policy-agent/opa/storage/inmem"
	"github.com/open-policy-agent/opa/topdown"
	"github.com/pkg/errors"
)

type LogLevel int

const (
	LogNone LogLevel = iota
	// Logs the output of Rego print() statements in the policy
	LogInfo
	// Logs the result objects returned from each query
	LogResults
	// Logs the full metadata state after each query
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
	// Logging
	logLevel       LogLevel
	logFile        *os.File
	infoLogger     *log.Logger
	resultsLogger  *log.Logger
	metadataLogger *log.Logger
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

// The result from a policy query
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
func NewRegoPolicyInterpreter(code string, inputData map[string]interface{}) (*RegoPolicyInterpreter, error) {
	data, err := copyObject(inputData)
	if err != nil {
		return nil, fmt.Errorf("unable to copy the input data: %w", err)
	}

	if _, ok := data["metadata"]; !ok {
		data["metadata"] = make(regoMetadata)
	}

	policy := &RegoPolicyInterpreter{
		code:     code,
		data:     data,
		modules:  make(map[string]*RegoModule),
		logLevel: LogNone,
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
		copy, err := copyValue(value)
		if err != nil {
			return nil, fmt.Errorf("unable to copy value: %w", err)
		}

		return copy, nil
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

func (r *RegoPolicyInterpreter) UpdateOSType(os string) error {
	r.dataAndModulesMutex.Lock()
	defer r.dataAndModulesMutex.Unlock()
	ops := []*regoMetadataOperation{
		{
			Action: metadataAdd,
			Name:   "operatingsystem",
			Key:    "ostype",
			Value:  os,
		},
	}
	return r.updateMetadata(ops)
}

func (r *RegoPolicyInterpreter) updateMetadata(ops []*regoMetadataOperation) error {
	// dataAndModulesMutex must be held before calling this

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
	r.logLevel = level

	file, err := os.OpenFile(path, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0666)
	if err != nil {
		return err
	}

	r.logFile = file
	r.infoLogger = log.New(file, "INFO: ", log.Ldate|log.Ltime)
	r.resultsLogger = log.New(file, "RESULT: ", log.Ldate|log.Ltime)
	r.metadataLogger = log.New(file, "METADATA: ", log.Ldate|log.Ltime)
	r.logInfo("Logging Enabled at level %d", level)

	return nil
}

// SetLogLevel sets the logging level. To actually produce a log, however, EnableLogging
// must be called first.
func (r *RegoPolicyInterpreter) SetLogLevel(level LogLevel) {
	// this mutex ensures no-one reads compiledModules before we clear it
	r.dataAndModulesMutex.Lock()
	defer r.dataAndModulesMutex.Unlock()

	r.compiledModules = nil
	r.logLevel = level
}

// DisableLogging disables logging and closes the underlying log file.
func (r *RegoPolicyInterpreter) DisableLogging() error {
	// this mutex ensures no-one reads compiledModules before we clear it
	r.dataAndModulesMutex.Lock()
	defer r.dataAndModulesMutex.Unlock()

	r.compiledModules = nil
	r.logLevel = LogNone
	if r.logFile != nil {
		r.logInfo("Logging disabled")
		r.infoLogger = nil
		r.resultsLogger = nil
		r.metadataLogger = nil
		err := r.logFile.Close()
		if err != nil {
			return err
		}

		r.logFile = nil
	}

	return nil
}

func (r *RegoPolicyInterpreter) compile() error {
	// dataAndModulesMutex must be held before calling this

	modules := make(map[string]string)
	for _, module := range r.modules {
		modules[module.Namespace+".rego"] = module.Code
	}

	modules["policy.rego"] = r.code

	options := ast.CompileOpts{
		EnablePrintStatements: r.logLevel != LogNone,
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

func (r *RegoPolicyInterpreter) logInfo(message string, args ...interface{}) {
	if r.logLevel < LogInfo || len(message) == 0 {
		return
	}
	r.infoLogger.Printf(message, args...)
}

func (r *RegoPolicyInterpreter) logResult(rule string, resultSet interface{}) {
	if r.logLevel < LogResults {
		return
	}

	contents, err := json.Marshal(resultSet)
	if err != nil {
		r.resultsLogger.Printf("error marshaling result set: %v\n", err.Error())
	} else {
		r.resultsLogger.Printf("%s -> %s", rule, string(contents))
	}
}

func (r *RegoPolicyInterpreter) logMetadata() {
	if r.logLevel < LogMetadata {
		return
	}

	contents, err := json.Marshal(r.data["metadata"])
	if err != nil {
		r.metadataLogger.Printf("error marshaling metadata: %v\n", err.Error())
	} else {
		r.metadataLogger.Println(string(contents))
	}
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

func (r *RegoPolicyInterpreter) query(rule string, input map[string]interface{}) (rego.ResultSet, error) {
	// dataAndModulesMutex must be held before calling this

	store := inmem.NewFromObject(r.data)

	var buf bytes.Buffer
	query := rego.New(
		rego.Query(rule),
		rego.Input(input),
		rego.Store(store),
		rego.EnablePrintStatements(r.logLevel != LogNone),
		rego.PrintHook(topdown.NewPrintHook(&buf)),
		rego.Compiler(r.compiledModules))

	ctx := context.Background()
	resultSet, err := query.Eval(ctx)
	output := buf.String()

	r.logInfo("%s", output)

	return resultSet, err
}

func (r *RegoPolicyInterpreter) RawQuery(rule string, input map[string]interface{}) (rego.ResultSet, error) {
	// this mutex ensures no other threads modify the data and compiledModules fields during query execution
	r.dataAndModulesMutex.Lock()
	defer r.dataAndModulesMutex.Unlock()

	if r.compiledModules == nil {
		err := r.compile()
		if err != nil {
			return nil, fmt.Errorf("error when compiling modules: %w", err)
		}
	}

	resultSet, err := r.query(rule, input)

	if err != nil {
		return nil, err
	}

	r.logResult(rule, resultSet)

	return resultSet, nil
}

// Query queries the policy with the given rule and input data and returns the result.
func (r *RegoPolicyInterpreter) Query(rule string, input map[string]interface{}) (RegoQueryResult, error) {
	// this mutex ensures no other threads modify the data and compiledModules fields during query execution
	r.dataAndModulesMutex.Lock()
	defer r.dataAndModulesMutex.Unlock()

	if r.compiledModules == nil {
		err := r.compile()
		if err != nil {
			return nil, fmt.Errorf("error when compiling modules: %w", err)
		}
	}

	rawResult, err := r.query(rule, input)

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

	r.logResult(rule, resultSet)

	ops := []*regoMetadataOperation{}
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
