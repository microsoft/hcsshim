package regopolicyinterpreter

import (
	_ "embed"
	"errors"
	"fmt"
	"math/rand"
	"os"
	"reflect"
	"strconv"
	"testing"
	"testing/quick"
	"time"
)

const (
	maxValue          = 1000
	maxNumberOfPairs  = 30
	stringLength      = 10
	maxNumberOfFields = 30
	maxArrayLength    = 10
	maxObjectDepth    = 4
)

var testRand *rand.Rand
var uniqueStrings map[string]struct{}

func init() {
	seed := time.Now().Unix()
	if seedStr, ok := os.LookupEnv("SEED"); ok {

		if parsedSeed, err := strconv.ParseInt(seedStr, 10, 64); err != nil {
			fmt.Fprintf(os.Stderr, "failed to parse seed: %d\n", seed)
		} else {
			seed = parsedSeed
		}
	}
	testRand = rand.New(rand.NewSource(seed))
	fmt.Fprintf(os.Stdout, "regopolicyinterpreter_test seed: %d\n", seed)
	uniqueStrings = make(map[string]struct{})
}

func Test_copyObject(t *testing.T) {
	f := func(orig testObject) bool {
		copy, err := copyObject(orig)
		if err != nil {
			t.Error(err)
			return false
		}

		return assertObjectsEqual(orig, copy)
	}

	if err := quick.Check(f, &quick.Config{MaxCount: 1000, Rand: testRand}); err != nil {
		t.Errorf("Test_copyObject: %v", err)
	}
}

func Test_copyValue(t *testing.T) {
	f := func(orig testValue) bool {
		valueCopy, err := copyValue(orig.value)
		if err != nil {
			t.Error(err)
			return false
		}

		return assertValuesEqual(orig.value, valueCopy)
	}

	if err := quick.Check(f, &quick.Config{MaxCount: 1000, Rand: testRand}); err != nil {
		t.Errorf("Test_copyValue: %v", err)
	}
}

//go:embed test.rego
var testCode string

func Test_Bool(t *testing.T) {
	rego, err := setupRego()
	if err != nil {
		t.Fatal(err)
	}

	f := func(p intPair) bool {
		result, err := getResult(rego, p, "is_greater_than")
		if err != nil {
			t.Error(err)
			return false
		}

		expected := p.a >= p.b
		actual, err := result.Bool("result")
		if err != nil {
			t.Error(err)
			return false
		}

		if actual != expected {
			actualRaw, _ := result.Value("result")
			t.Errorf("received unexpected result: %d >= %d = %v", p.a, p.b, actualRaw)
			return false
		}

		return true
	}

	if err := quick.Check(f, &quick.Config{MaxCount: 1000, Rand: testRand}); err != nil {
		t.Errorf("Test_Bool: %v", err)
	}
}

func Test_Int(t *testing.T) {
	rego, err := setupRego()
	if err != nil {
		t.Fatal(err)
	}

	f := func(p intPair) bool {
		result, err := getResult(rego, p, "add")
		if err != nil {
			t.Error(err)
			return false
		}

		expected := p.a + p.b
		actual, err := result.Int("result")
		if err != nil {
			t.Errorf("error retrieving int value: %v", err)
			return false
		}

		if actual != expected {
			t.Errorf("received unexpected result: %d + %d = %d", p.a, p.b, actual)
			return false
		}

		return true
	}

	if err := quick.Check(f, &quick.Config{MaxCount: 1000, Rand: testRand}); err != nil {
		t.Errorf("Test_Int: %v", err)
	}
}

func Test_Float(t *testing.T) {
	rego, err := setupRego()
	if err != nil {
		t.Fatal(err)
	}

	f := func(ip intPair) bool {
		p := ip.toFloat()
		input := map[string]interface{}{"a": p.a, "b": p.b}
		result, err := rego.Query("data.test.add", input)
		if err != nil {
			t.Errorf("received error when trying to query rego: %v", err)
			return false
		}

		if result.IsEmpty() {
			t.Error("received empty result from query")
			return false
		}

		expected := p.a + p.b
		actual, err := result.Float("result")
		if err != nil {
			t.Errorf("error retrieving int value: %v", err)
			return false
		}

		if !approxEqual(actual, expected) {
			t.Errorf("received unexpected result: %f + %f = %f", p.a, p.b, actual)
			return false
		}

		return true
	}

	if err := quick.Check(f, &quick.Config{MaxCount: 1000, Rand: testRand}); err != nil {
		t.Errorf("Test_Int: %v", err)
	}
}

func Test_String(t *testing.T) {
	rego, err := setupRego()
	if err != nil {
		t.Fatal(err)
	}

	f := func(ip intPair) bool {
		p := ip.toString()
		input := map[string]interface{}{"a": p.a, "b": p.b}
		result, err := rego.Query("data.test.add", input)
		if err != nil {
			t.Errorf("received error when trying to query rego: %v", err)
			return false
		}

		if result.IsEmpty() {
			t.Error("received empty result from query")
			return false
		}

		expected := p.a + "+" + p.b
		actual, err := result.String("result")

		if err != nil {
			t.Error(err)
			return false
		}

		if actual != expected {
			t.Errorf("received unexpected result: %s = %s", expected, actual)
			return false
		}

		return true
	}

	if err := quick.Check(f, &quick.Config{MaxCount: 1000, Rand: testRand}); err != nil {
		t.Errorf("Test_Int: %v", err)
	}
}

func Test_Metadata_Add(t *testing.T) {
	rego, err := setupRego()
	if err != nil {
		t.Fatal(err)
	}

	f := func(p intPair, name metadataName) bool {
		err = createLists(rego, p, name)
		if err != nil {
			t.Error(err)
			return false
		}

		greater := make([]int, 1)
		lesser := make([]int, 1)
		if p.a >= p.b {
			greater[0] = p.a
			lesser[0] = p.b
		} else {
			greater[0] = p.b
			lesser[0] = p.a
		}

		err = assertListEqual(rego, name, "lesser", lesser)
		if err != nil {
			t.Error(err)
			return false
		}

		err = assertListEqual(rego, name, "greater", greater)
		if err != nil {
			t.Error(err)
			return false
		}

		err = createLists(rego, p, name)
		if err == nil {
			t.Errorf("did not expect to be able to call create twice")
			return false
		}

		return true
	}

	if err := quick.Check(f, &quick.Config{MaxCount: 100, Rand: testRand}); err != nil {
		t.Errorf("Test_Metadata_Add: %v", err)
	}
}

func Test_Metadata_Update(t *testing.T) {
	rego, err := setupRego()
	if err != nil {
		t.Fatal(err)
	}

	f := func(pairs intPairArray, name metadataName) bool {
		greater := make([]int, len(pairs))
		lesser := make([]int, len(pairs))

		for i, pair := range pairs {
			err = appendLists(rego, pair, name)
			if err != nil {
				t.Error(err)
				return false
			}

			if pair.a >= pair.b {
				greater[i], lesser[i] = pair.a, pair.b
			} else {
				greater[i], lesser[i] = pair.b, pair.a
			}
		}

		err = assertListEqual(rego, name, "lesser", lesser)
		if err != nil {
			t.Error(err)
			return false
		}

		err = assertListEqual(rego, name, "greater", greater)
		if err != nil {
			t.Error(err)
			return false
		}

		err = createLists(rego, pairs[0], name)
		if err == nil {
			t.Errorf("did not expect to be able to call create after update")
			return false
		}

		return true
	}

	if err := quick.Check(f, &quick.Config{MaxCount: 100, Rand: testRand}); err != nil {
		t.Errorf("Test_Metadata_Update: %v", err)
	}
}

func Test_Metadata_Remove(t *testing.T) {
	rego, err := setupRego()
	if err != nil {
		t.Fatal(err)
	}

	f := func(pairs intPairArray, name metadataName) bool {
		expected := 0
		for _, pair := range pairs {
			err = appendLists(rego, pair, name)
			if err != nil {
				t.Error(err)
				return false
			}

			if pair.a >= pair.b {
				expected += pair.a - pair.b
			} else {
				expected += pair.b - pair.a
			}
		}

		err = computeGap(rego, name, expected)
		if err != nil {
			t.Error(err)
			return false
		}

		err = createLists(rego, pairs[0], name)
		if err != nil {
			t.Errorf("expected to be able to call create after compute_gap")
			return false
		}

		return true
	}

	if err := quick.Check(f, &quick.Config{MaxCount: 100, Rand: testRand}); err != nil {
		t.Errorf("Test_Metadata_Remove: %v", err)
	}
}

//go:embed module.rego
var moduleCode string

func Test_Module(t *testing.T) {
	rego, err := setupRego()
	if err != nil {
		t.Fatal(err)
	}

	rego.AddModule("module.rego", &RegoModule{Namespace: "module", Code: moduleCode})

	if rego.compiledModules != nil {
		t.Fatal("adding a module should clear the compiled artifacts")
	}

	f := func(p intPair) bool {
		result, err := getResult(rego, p, "subtract")
		if err != nil {
			t.Error(err)
			return false
		}

		expected := p.a - p.b
		if actual, err := result.Int("result"); err == nil {
			if actual != expected {
				t.Errorf("received invalid result %d - %d = %d", p.a, p.b, actual)
				return false
			}
		} else {
			t.Errorf("error retrieving result: %v", err)
			return false
		}

		return true
	}

	if err = quick.Check(f, &quick.Config{MaxCount: 100, Rand: testRand}); err != nil {
		t.Errorf("Test_Module_Compiled: %v", err)
	}

	rego.RemoveModule("module.rego")

	if _, ok := rego.modules["module.rego"]; ok {
		t.Errorf("Module still present after remove")
	}

	if rego.compiledModules != nil {
		t.Errorf("removing a module should clear the compiled artifacts")
	}

	p := generateIntPair(testRand)
	if _, err := getResult(rego, p, "subtract"); err == nil {
		t.Errorf("able to call subtract after module has been removed")
	}

}

// fixtures

func setupRego() (*RegoPolicyInterpreter, error) {
	rego, err := NewRegoPolicyInterpreter(testCode, map[string]interface{}{})
	if err != nil {
		return nil, fmt.Errorf("unable to create an interpreter: %w", err)
	}

	err = rego.Compile()
	if err != nil {
		return nil, fmt.Errorf("unable to compile rego: %w", err)
	}

	return rego, nil
}

func approxEqual(a float64, b float64) bool {
	diff := a - b
	if diff < 0 {
		diff = -diff
	}
	return diff < 1e-5
}

type intPair struct {
	a int
	b int
}

type intPairArray []intPair

type floatPair struct {
	a float64
	b float64
}

type stringPair struct {
	a string
	b string
}

func (p intPair) toFloat() floatPair {
	return floatPair{
		a: float64(p.a) / float64(maxValue),
		b: float64(p.b) / float64(maxValue),
	}
}

func (p intPair) toString() stringPair {
	return stringPair{
		a: strconv.Itoa(p.a),
		b: strconv.Itoa(p.b),
	}
}

func generateIntPair(r *rand.Rand) intPair {
	a := r.Intn(maxValue) + 1
	b := maxValue - a
	return intPair{a, b}
}

func (intPair) Generate(r *rand.Rand, _ int) reflect.Value {
	p := generateIntPair(r)
	return reflect.ValueOf(p)
}

func (intPairArray) Generate(r *rand.Rand, _ int) reflect.Value {
	numPairs := r.Intn(maxNumberOfPairs-10) + 10
	pairs := make([]intPair, numPairs)
	for i := 0; i < numPairs; i++ {
		pairs[i] = generateIntPair(r)
	}
	return reflect.ValueOf(pairs)
}

type metadataName string

func (metadataName) Generate(r *rand.Rand, _ int) reflect.Value {
	value := metadataName(uniqueString(r))
	return reflect.ValueOf(value)
}

type testValue struct {
	value interface{}
}
type testArray []interface{}
type testObject map[string]interface{}

type testValueType int

const (
	testValueObject testValueType = iota
	testValueArray
	testValueString
	testValueFloat
	testValueBool
	testValueNull
)

func generateValue(r *rand.Rand, depth int) interface{} {
	choices := []testValueType{testValueArray, testValueString, testValueFloat, testValueBool, testValueNull}
	if depth < maxObjectDepth {
		choices = append(choices, testValueObject)
	}

	switch choices[r.Intn(len(choices))] {
	case testValueObject:
		return generateObject(r, depth+1)

	case testValueArray:
		return generateArray(r, depth+1)

	case testValueString:
		return randString(r)

	case testValueFloat:
		return r.Float64()

	case testValueBool:
		return r.Intn(2) == 1

	case testValueNull:
		return nil

	default:
		panic("invalid test value type")
	}
}

func generateArray(r *rand.Rand, depth int) testArray {
	numElements := r.Intn(maxArrayLength)
	values := make(testArray, numElements)
	for i := 0; i < numElements; i++ {
		values[i] = generateValue(r, depth+1)
	}
	return values
}

func generateObject(r *rand.Rand, depth int) testObject {
	result := make(testObject)
	numFields := r.Intn(maxNumberOfFields)
	for f := 0; f < numFields; f++ {
		name := uniqueString(r)
		result[name] = generateValue(r, depth)
	}

	return result
}

func (testValue) Generate(r *rand.Rand, _ int) reflect.Value {
	value := testValue{value: generateValue(r, 0)}
	return reflect.ValueOf(value)
}

func (testObject) Generate(r *rand.Rand, _ int) reflect.Value {
	value := generateObject(r, 0)
	return reflect.ValueOf(value)
}

func getResult(r *RegoPolicyInterpreter, p intPair, rule string) (RegoQueryResult, error) {
	input := map[string]interface{}{"a": p.a, "b": p.b}
	result, err := r.Query("data.test."+rule, input)
	if err != nil {
		return nil, fmt.Errorf("received error when trying to query rego: %w", err)
	}

	if result.IsEmpty() {
		return nil, errors.New("received empty result from query")
	}

	return result, nil
}

func createLists(r *RegoPolicyInterpreter, p intPair, name metadataName) error {
	input := map[string]interface{}{"a": p.a, "b": p.b, "name": string(name)}
	result, err := r.Query("data.test.create", input)
	if err != nil {
		return fmt.Errorf("received error when trying to query rego: %w", err)
	}

	if result.IsEmpty() {
		return errors.New("received empty result from query")
	}

	success, err := result.Bool("success")
	if err != nil {
		return err
	}

	if !success {
		return errors.New("create query failed unexpectedly")
	}

	return nil
}

func appendLists(r *RegoPolicyInterpreter, p intPair, name metadataName) error {
	input := map[string]interface{}{"a": p.a, "b": p.b, "name": string(name)}
	result, err := r.Query("data.test.append", input)
	if err != nil {
		return fmt.Errorf("received error when trying to query rego: %w", err)
	}

	if result.IsEmpty() {
		return errors.New("received empty result from query")
	}

	success, err := result.Bool("success")
	if err != nil {
		return err
	}

	if !success {
		return errors.New("update query failed unexpectedly")
	}

	return nil
}

func computeGap(r *RegoPolicyInterpreter, name metadataName, expected int) error {
	input := map[string]interface{}{"name": string(name)}
	result, err := r.Query("data.test.compute_gap", input)
	if err != nil {
		return fmt.Errorf("received error when trying to query rego: %w", err)
	}

	if result.IsEmpty() {
		return errors.New("received empty result from query")
	}

	actual, err := result.Int("result")
	if err != nil {
		return fmt.Errorf("error obtaining result: %w", err)
	}

	if actual != expected {
		return fmt.Errorf("expected %d, received %d", expected, actual)
	}

	return nil
}

func assertListEqual(r *RegoPolicyInterpreter, name metadataName, key string, expectedValues []int) error {
	rawValues, err := r.GetMetadata(string(name), key)
	if err != nil {
		return fmt.Errorf("unable to get metadata list %s: %w", name, err)
	}

	actualValues := make([]int, len(expectedValues))
	if values, ok := rawValues.([]interface{}); ok {
		if len(values) != len(expectedValues) {
			return fmt.Errorf("incorrect array length: %d != %d", len(values), len(expectedValues))
		}

		for i, value := range values {
			if number, ok := value.(float64); ok {
				actualValues[i] = int(number)
			} else {
				return fmt.Errorf("cannot cast %v to float64", value)
			}
		}
	} else {
		return errors.New("cannot cast raw values to array")
	}

	for i, actual := range actualValues {
		if actual != expectedValues[i] {
			return fmt.Errorf("a[%d] != e[%d] (%d != %d)", i, i, actual, expectedValues[i])
		}
	}

	return nil
}

func randChar(r *rand.Rand) byte {
	charset := "abcdefghijklmnopqrstuvwxyzABCDEFGHIJKLMNOPQRSTUVWXYZ"
	return charset[r.Intn(len(charset))]
}

func randString(r *rand.Rand) string {
	charset := "abcdefghijklmnopqrstuvwxyzABCDEFGHIJKLMNOPQRSTUVWXYZ0123456789"
	chars := make([]byte, stringLength)
	chars[0] = randChar(r)
	for i := 1; i < stringLength; i++ {
		chars[i] = charset[r.Intn(len(charset))]
	}
	return string(chars)
}

func uniqueString(r *rand.Rand) string {
	for {
		s := randString(r)

		if _, ok := uniqueStrings[s]; !ok {
			uniqueStrings[s] = struct{}{}
			return s
		}
	}
}

func assertValuesEqual(lhs interface{}, rhs interface{}) bool {
	if lhsObject, ok := lhs.(testObject); ok {
		if rhsObject, ok := rhs.(testObject); ok {
			return assertObjectsEqual(lhsObject, rhsObject)
		} else if rhsObject, ok := rhs.(map[string]interface{}); ok {
			return assertObjectsEqual(lhsObject, rhsObject)
		} else {
			return false
		}
	}

	if lhsArray, ok := lhs.(testArray); ok {
		if rhsArray, ok := rhs.(testArray); ok {
			return assertArraysEqual(lhsArray, rhsArray)
		} else if rhsArray, ok := rhs.([]interface{}); ok {
			return assertArraysEqual(lhsArray, rhsArray)
		} else {
			return false
		}
	}

	if lhsString, ok := lhs.(string); ok {
		if rhsString, ok := rhs.(string); ok {
			return lhsString == rhsString
		} else {
			return false
		}
	}

	if lhsFloat, ok := lhs.(float64); ok {
		if rhsFloat, ok := rhs.(float64); ok {
			return lhsFloat == rhsFloat
		} else {
			return false
		}
	}

	if lhsBool, ok := lhs.(bool); ok {
		if rhsBool, ok := rhs.(bool); ok {
			return lhsBool == rhsBool
		} else {
			return false
		}
	}

	if lhs == nil && rhs == nil {
		return true
	}

	return false
}

func assertArraysEqual(lhs testArray, rhs testArray) bool {
	if len(lhs) != len(rhs) {
		return false
	}

	for i := range lhs {
		if !assertValuesEqual(lhs[i], rhs[i]) {
			return false
		}
	}

	return true
}

func assertObjectsEqual(lhs testObject, rhs testObject) bool {
	if len(lhs) != len(rhs) {
		return false
	}

	for key, lhsValue := range lhs {
		if rhsValue, ok := lhs[key]; ok {
			if !assertValuesEqual(lhsValue, rhsValue) {
				return false
			}
		} else {
			return false
		}
	}

	return true
}
