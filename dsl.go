// Copyright 2015 Peter Goetz
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//     http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package pegomock

import (
	"fmt"
	"reflect"
	"testing"

	"github.com/petergtz/pegomock/internal/verify"
)

var GlobalFailHandler FailHandler

func RegisterMockFailHandler(handler FailHandler) {
	GlobalFailHandler = handler
}
func RegisterMockTestingT(t *testing.T) {
	RegisterMockFailHandler(BuildTestingTGomegaFailHandler(t))
}

var lastInvocation *invocation
var globalArgMatchers Matchers

func RegisterMatcher(matcher Matcher) {
	globalArgMatchers.append(matcher)
}

type invocation struct {
	genericMock *GenericMock
	MethodName  string
	Params      []Param
	ReturnTypes []reflect.Type
	IsVariadic  bool
}

type GenericMock struct {
	mockedMethods map[string]*mockedMethod
}

func (genericMock *GenericMock) Invoke(methodName string, params []Param, isVariadic bool, returnTypes []reflect.Type) ReturnValues {
	lastInvocation = &invocation{
		genericMock: genericMock,
		MethodName:  methodName,
		Params:      params,
		ReturnTypes: returnTypes,
		IsVariadic:  isVariadic,
	}
	return genericMock.getOrCreateMockedMethod(methodName).Invoke(params)
}

func (genericMock *GenericMock) stub(methodName string, paramMatchers []Matcher, returnValues ReturnValues) {
	genericMock.stubWithCallback(methodName, paramMatchers, func([]Param) ReturnValues { return returnValues })
}

func (genericMock *GenericMock) stubWithCallback(methodName string, paramMatchers []Matcher, callback func([]Param) ReturnValues) {
	genericMock.getOrCreateMockedMethod(methodName).stub(paramMatchers, callback)
}

func (genericMock *GenericMock) getOrCreateMockedMethod(methodName string) *mockedMethod {
	if _, ok := genericMock.mockedMethods[methodName]; !ok {
		genericMock.mockedMethods[methodName] = &mockedMethod{name: methodName}
	}
	return genericMock.mockedMethods[methodName]
}

func (genericMock *GenericMock) reset(methodName string, paramMatchers []Matcher) {
	genericMock.getOrCreateMockedMethod(methodName).reset(paramMatchers)
}

func (genericMock *GenericMock) Verify(
	inOrderContext *InOrderContext,
	invocationCountMatcher Matcher,
	methodName string,
	params []Param,
	isVariadic bool) {
	if GlobalFailHandler == nil {
		panic("No GlobalFailHandler set. Please use either RegisterMockFailHandler or RegisterMockTestingT to set a fail handler.")
	}
	defer func() { globalArgMatchers = nil }() // We don't want a panic somewhere during verification screw our global argMatchers

	// FIXME: should manipulate globalArgMatchers to group variadic part into a SliceMatcher made up of the individual variadic arg matchers

	if len(globalArgMatchers) != 0 {
		if isVariadic {
			globalArgMatchers = groupVariadicPartIntoSliceMatcher(globalArgMatchers, len(params))
		}
		verifyArgMatcherUse(globalArgMatchers, params, isVariadic)
	}

	methodInvocations := genericMock.methodInvocations(methodName, params, globalArgMatchers)
	if inOrderContext != nil {
		for _, methodInvocation := range methodInvocations {
			if methodInvocation.orderingInvocationNumber <= inOrderContext.invocationCounter {
				GlobalFailHandler(fmt.Sprintf("Expected function call \"%v\" with params %v before function call \"%v\" with params %v",
					methodName, params, inOrderContext.lastInvokedMethodName, inOrderContext.lastInvokedMethodParams))
			}
			inOrderContext.invocationCounter = methodInvocation.orderingInvocationNumber
			inOrderContext.lastInvokedMethodName = methodName
			inOrderContext.lastInvokedMethodParams = params
		}
	}
	if !invocationCountMatcher.Matches(len(methodInvocations)) {
		if len(globalArgMatchers) == 0 {
			GlobalFailHandler(fmt.Sprintf(
				"Mock invocation count for method \"%s\" with params %v does not match expectation.\n\n\t%v",
				methodName, params, invocationCountMatcher.FailureMessage()))
		} else {
			GlobalFailHandler(fmt.Sprintf(
				"Mock invocation count for method \"%s\" with params %v does not match expectation.\n\n\t%v",
				methodName, globalArgMatchers, invocationCountMatcher.FailureMessage()))
		}
	}
}

type SliceMatcher struct {
}

func (matcher *SliceMatcher) Matches(param Param) bool {
	return false
}

func (matcher *SliceMatcher) FailureMessage() string {
	return ""
}

func (matcher *SliceMatcher) String() string {
	return ""
}

func groupVariadicPartIntoSliceMatcher(matchers Matchers, numRegularParams int) Matchers {
	result := make([]Matcher, numRegularParams+1)
	for i := 0; i < numRegularParams; i++ {
		result[i] = matchers[i]
	}
	result[numRegularParams] = &SliceMatcher{}
	return result
}

func (genericMock *GenericMock) GetInvocationParams(methodName string) [][]Param {
	if len(genericMock.mockedMethods[methodName].invocations) == 0 {
		return nil
	}
	result := make([][]Param, len(genericMock.mockedMethods[methodName].invocations[len(genericMock.mockedMethods[methodName].invocations)-1].params))
	for _, invocation := range genericMock.mockedMethods[methodName].invocations {
		for u, param := range invocation.params {
			result[u] = append(result[u], param)
		}
	}
	return result
}

func (genericMock *GenericMock) methodInvocations(methodName string, params []Param, matchers []Matcher) []methodInvocation {
	if len(matchers) != 0 {
		return genericMock.methodInvocationsUsingMatchers(methodName, matchers)
	}

	invocations := make([]methodInvocation, 0)
	if _, exists := genericMock.mockedMethods[methodName]; exists {
		for _, invocation := range genericMock.mockedMethods[methodName].invocations {
			if reflect.DeepEqual(params, invocation.params) {
				invocations = append(invocations, invocation)
			}
		}
	}
	return invocations
}

func (genericMock *GenericMock) methodInvocationsUsingMatchers(methodName string, paramMatchers Matchers) []methodInvocation {
	invocations := make([]methodInvocation, 0)
	for _, invocation := range genericMock.mockedMethods[methodName].invocations {
		if paramMatchers.Matches(invocation.params) {
			invocations = append(invocations, invocation)
		}
	}
	return invocations
}

type mockedMethod struct {
	name        string
	invocations []methodInvocation
	stubbings   Stubbings
}

func (method *mockedMethod) Invoke(params []Param) ReturnValues {
	method.invocations = append(method.invocations, methodInvocation{params, globalInvocationCounter.nextNumber()})
	stubbing := method.stubbings.find(params)
	if stubbing == nil {
		return ReturnValues{}
	}
	return stubbing.Invoke(params)
}

func (method *mockedMethod) stub(paramMatchers Matchers, callback func([]Param) ReturnValues) {
	stubbing := method.stubbings.findByMatchers(paramMatchers)
	if stubbing == nil {
		stubbing = &Stubbing{paramMatchers: paramMatchers}
		method.stubbings = append(method.stubbings, stubbing)
	}
	stubbing.callbackSequence = append(stubbing.callbackSequence, callback)
}

func (method *mockedMethod) removeLastInvocation() {
	method.invocations = method.invocations[:len(method.invocations)-1]
}

func (method *mockedMethod) reset(paramMatchers Matchers) {
	method.stubbings.removeByMatchers(paramMatchers)
}

type Counter struct {
	count int
}

func (counter *Counter) nextNumber() (nextNumber int) {
	nextNumber = counter.count
	counter.count++
	return
}

var globalInvocationCounter Counter

type methodInvocation struct {
	params                   []Param
	orderingInvocationNumber int
}

type Stubbings []*Stubbing

func (stubbings Stubbings) find(params []Param) *Stubbing {
	for i := len(stubbings) - 1; i >= 0; i-- {
		if stubbings[i].paramMatchers.Matches(params) {
			return stubbings[i]
		}
	}
	return nil
}

func (stubbings Stubbings) findByMatchers(paramMatchers Matchers) *Stubbing {
	for _, stubbing := range stubbings {
		if matchersEqual(stubbing.paramMatchers, paramMatchers) {
			return stubbing
		}
	}
	return nil
}

func (stubbings *Stubbings) removeByMatchers(paramMatchers Matchers) {
	for i, stubbing := range *stubbings {
		if matchersEqual(stubbing.paramMatchers, paramMatchers) {
			*stubbings = append((*stubbings)[:i], (*stubbings)[i+1:]...)
		}
	}
}

func matchersEqual(a, b Matchers) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if !reflect.DeepEqual(a[i], b[i]) {
			return false
		}
	}
	return true
}

type Stubbing struct {
	paramMatchers    Matchers
	callbackSequence []func([]Param) ReturnValues
	sequencePointer  int
}

func (stubbing *Stubbing) Invoke(params []Param) ReturnValues {
	defer func() {
		if stubbing.sequencePointer < len(stubbing.callbackSequence)-1 {
			stubbing.sequencePointer++
		}
	}()
	return stubbing.callbackSequence[stubbing.sequencePointer](params)
}

type Matchers []Matcher

func (matchers Matchers) Matches(params []Param) bool {
	verify.Argument(len(matchers) == len(params),
		"Number of params and matchers different: params: %v, matchers: %v",
		params, matchers)
	for i := range params {
		if !matchers[i].Matches(params[i]) {
			return false
		}
	}
	return true
}

func (matchers *Matchers) append(matcher Matcher) {
	*matchers = append(*matchers, matcher)
}

type ongoingStubbing struct {
	genericMock   *GenericMock
	MethodName    string
	ParamMatchers []Matcher
	returnTypes   []reflect.Type
}

func When(invocation ...interface{}) *ongoingStubbing {
	callIfIsFunc(invocation)
	verify.Argument(lastInvocation != nil,
		"When() requires an argument which has to be 'a method call on a mock'.")
	defer func() {
		lastInvocation = nil
		globalArgMatchers = nil
	}()
	lastInvocation.genericMock.mockedMethods[lastInvocation.MethodName].removeLastInvocation()

	paramMatchers := paramMatchersFromArgMatchersOrParams(globalArgMatchers, lastInvocation.Params, lastInvocation.IsVariadic)
	lastInvocation.genericMock.reset(lastInvocation.MethodName, paramMatchers)
	return &ongoingStubbing{
		genericMock:   lastInvocation.genericMock,
		MethodName:    lastInvocation.MethodName,
		ParamMatchers: paramMatchers,
		returnTypes:   lastInvocation.ReturnTypes,
	}
}

func callIfIsFunc(invocation []interface{}) {
	if len(invocation) == 1 {
		actualType := actualTypeOf(invocation[0])
		if actualType != nil && actualType.Kind() == reflect.Func && !reflect.ValueOf(invocation[0]).IsNil() {
			if !(actualType.NumIn() == 0 && actualType.NumOut() == 0) {
				panic("When using 'When' with function that does not return a value, " +
					"it expects a function with no arguments and no return value.")
			}
			reflect.ValueOf(invocation[0]).Call([]reflect.Value{})
		}
	}
}

// Deals with nils without panicking
func actualTypeOf(iface interface{}) reflect.Type {
	defer func() { recover() }()
	return reflect.TypeOf(iface)
}

func paramMatchersFromArgMatchersOrParams(argMatchers []Matcher, params []Param, isVariadic bool) []Matcher {
	if len(argMatchers) != 0 {
		verifyArgMatcherUse(argMatchers, params, isVariadic)
		return argMatchers
	}
	return transformParamsIntoEqMatchers(params)
}

func verifyArgMatcherUse(argMatchers []Matcher, params []Param, isVariadic bool) {
	verify.Argument(len(argMatchers) == len(params),
		"Invalid use of matchers!\n\n %v matchers expected, %v recorded.\n\n"+
			"This error may occur if matchers are combined with raw values:\n"+
			"    //incorrect:\n"+
			"    someFunc(AnyInt(), \"raw String\")\n"+
			"When using matchers, all arguments have to be provided by matchers.\n"+
			"For example:\n"+
			"    //correct:\n"+
			"    someFunc(AnyInt(), EqString(\"String by matcher\"))",
		len(params), len(argMatchers),
	)
}

func transformParamsIntoEqMatchers(params []Param) []Matcher {
	paramMatchers := make([]Matcher, len(params))
	for i, param := range params {
		paramMatchers[i] = &EqMatcher{Value: param}
	}
	return paramMatchers
}

var genericMocks = make(map[Mock]*GenericMock)

func GetGenericMockFrom(mock Mock) *GenericMock {
	if genericMocks[mock] == nil {
		genericMocks[mock] = &GenericMock{mockedMethods: make(map[string]*mockedMethod)}
	}
	return genericMocks[mock]
}

func (stubbing *ongoingStubbing) ThenReturn(values ...ReturnValue) *ongoingStubbing {
	checkAssignabilityOf(values, stubbing.returnTypes)
	stubbing.genericMock.stub(stubbing.MethodName, stubbing.ParamMatchers, values)
	return stubbing
}

func checkAssignabilityOf(stubbedReturnValues []ReturnValue, expectedReturnTypes []reflect.Type) {
	verify.Argument(len(stubbedReturnValues) == len(expectedReturnTypes),
		"Different number of return values")
	for i := range stubbedReturnValues {
		if stubbedReturnValues[i] == nil {
			switch expectedReturnTypes[i].Kind() {
			case reflect.Bool, reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64, reflect.Uint,
				reflect.Uint8, reflect.Uint16, reflect.Uint32, reflect.Uint64, reflect.Uintptr, reflect.Float32,
				reflect.Float64, reflect.Complex64, reflect.Complex128, reflect.Array, reflect.String,
				reflect.Struct:
				panic("Return value 'nil' not assignable to return type " + expectedReturnTypes[i].Kind().String())
			}
		} else {
			verify.Argument(reflect.TypeOf(stubbedReturnValues[i]).AssignableTo(expectedReturnTypes[i]),
				"Return value of type %T not assignable to return type %v", stubbedReturnValues[i], expectedReturnTypes[i])
		}
	}
}

func (stubbing *ongoingStubbing) ThenPanic(v interface{}) *ongoingStubbing {
	stubbing.genericMock.stubWithCallback(
		stubbing.MethodName,
		stubbing.ParamMatchers,
		func([]Param) ReturnValues { panic(v) })
	return stubbing
}

func (stubbing *ongoingStubbing) Then(callback func([]Param) ReturnValues) *ongoingStubbing {
	stubbing.genericMock.stubWithCallback(
		stubbing.MethodName,
		stubbing.ParamMatchers,
		callback)
	return stubbing
}

type InOrderContext struct {
	invocationCounter       int
	lastInvokedMethodName   string
	lastInvokedMethodParams []Param
}

type Stubber struct {
	returnValue interface{}
}

func DoPanic(value interface{}) *Stubber {
	return &Stubber{returnValue: value}
}

func (stubber *Stubber) When(mock interface{}) {

}

// Matcher ... it is guaranteed that FailureMessage will always be called after Matches
// so an implementation can save state
type Matcher interface {
	Matches(param Param) bool
	FailureMessage() string
	fmt.Stringer
}
