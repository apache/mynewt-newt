/**
 * Licensed to the Apache Software Foundation (ASF) under one
 * or more contributor license agreements.  See the NOTICE file
 * distributed with this work for additional information
 * regarding copyright ownership.  The ASF licenses this file
 * to you under the Apache License, Version 2.0 (the
 * "License"); you may not use this file except in compliance
 * with the License.  You may obtain a copy of the License at
 *
 *  http://www.apache.org/licenses/LICENSE-2.0
 *
 * Unless required by applicable law or agreed to in writing,
 * software distributed under the License is distributed on an
 * "AS IS" BASIS, WITHOUT WARRANTIES OR CONDITIONS OF ANY
 * KIND, either express or implied.  See the License for the
 * specific language governing permissions and limitations
 * under the License.
 */

package yaml

import (
	"errors"
	"fmt"
	"strconv"
)

type decodeState int

const (
	CTXT_STATE_NONE     decodeState = iota
	CTXT_STATE_SCALAR   decodeState = iota
	CTXT_STATE_MAPPING  decodeState = iota
	CTXT_STATE_SEQUENCE decodeState = iota
	CTXT_STATE_DONE     decodeState = iota
)

type decodeCtxt struct {
	state decodeState
	value interface{}
}

type YamlDispatchFn func(*yaml_parser_t, *yaml_event_t,
	*decodeCtxt) (decodeCtxt, error)

var decodeFilename string
var decodeDispatch map[yaml_event_type_t]YamlDispatchFn

// Fills in the decodeDispatch table.  This function is necessary because
// statically initializing the table triggers a spurious "initialization loop"
// error.
func initDecodeDispatch() {
	if decodeDispatch != nil {
		return
	}

	decodeDispatch = map[yaml_event_type_t]YamlDispatchFn{
		yaml_STREAM_START_EVENT:   decodeNoOp,
		yaml_DOCUMENT_START_EVENT: decodeNoOp,
		yaml_DOCUMENT_END_EVENT:   decodeNoOp,
		yaml_ALIAS_EVENT:          decodeNoOp,
		yaml_STREAM_END_EVENT:     decodeStreamEnd,
		yaml_SCALAR_EVENT:         decodeScalar,
		yaml_SEQUENCE_START_EVENT: decodeSequenceStart,
		yaml_SEQUENCE_END_EVENT:   decodeSequenceEnd,
		yaml_MAPPING_START_EVENT:  decodeMappingStart,
		yaml_MAPPING_END_EVENT:    decodeMappingEnd,
	}
}

func decodeError(parser *yaml_parser_t, format string,
	sprintfArgs ...interface{}) error {

	prefix := fmt.Sprintf("[%s:%d]: ", decodeFilename, parser.mark.line+1)
	msg := prefix + fmt.Sprintf(format, sprintfArgs...)
	return errors.New(msg)
}

// Appends the specified value to the end of a sequence-context's value slice.
func sequenceAdd(ctxt *decodeCtxt, value interface{}) {
	if ctxt.value == nil {
		ctxt.value = []interface{}{}
	}
	ctxt.value = append(ctxt.value.([]interface{}), value)
}

// Inserts the specified value into a mapping-context's value map.
func mappingAdd(ctxt *decodeCtxt, key string, value interface{}) {
	if ctxt.value == nil {
		ctxt.value = map[interface{}]interface{}{}
	}

	ctxt.value.(map[interface{}]interface{})[key] = value
}

func genValue(strVal string) interface{} {
	intVal, err := strconv.Atoi(strVal)
	if err == nil {
		return intVal
	}

	boolVal, err := strconv.ParseBool(strVal)
	if err == nil {
		return boolVal
	}

	return strVal
}

func stringValue(value interface{}) string {
	switch value.(type) {
	case int:
		return strconv.FormatInt(int64(value.(int)), 10)

	case bool:
		return strconv.FormatBool(value.(bool))

	case string:
		return value.(string)

	default:
		panic("unexpected type")
	}
}

func decodeNoOp(parser *yaml_parser_t, event *yaml_event_t,
	parentCtxt *decodeCtxt) (decodeCtxt, error) {

	return decodeCtxt{state: CTXT_STATE_NONE}, nil
}

func decodeStreamEnd(parser *yaml_parser_t, event *yaml_event_t,
	parentCtxt *decodeCtxt) (decodeCtxt, error) {

	return decodeCtxt{state: CTXT_STATE_DONE}, nil
}

func decodeNextValue(parser *yaml_parser_t,
	parentCtxt *decodeCtxt) (decodeCtxt, error) {
	for {
		ctxt, err := decodeEvent(parser, parentCtxt)
		if err != nil {
			return ctxt, err
		}

		if ctxt.state != CTXT_STATE_NONE {
			return ctxt, nil
		}
	}
}

func decodeSequenceStart(parser *yaml_parser_t, event *yaml_event_t,
	parentCtxt *decodeCtxt) (decodeCtxt, error) {

	ctxt := decodeCtxt{state: CTXT_STATE_SEQUENCE}

	// For each element in the sequence, decode it and append it to the
	// seqeunce-context's value slice.
	for {
		subCtxt, err := decodeNextValue(parser, &ctxt)
		if err != nil {
			return ctxt, err
		}

		if subCtxt.state == CTXT_STATE_DONE {
			break
		}

		sequenceAdd(&ctxt, subCtxt.value)
	}

	return ctxt, nil
}

func decodeSequenceEnd(parser *yaml_parser_t, event *yaml_event_t,
	parentCtxt *decodeCtxt) (decodeCtxt, error) {

	ctxt := decodeCtxt{state: CTXT_STATE_DONE}
	if parentCtxt.state != CTXT_STATE_SEQUENCE {
		return ctxt, decodeError(parser, "sequence end without start")
	}

	return ctxt, nil
}

func decodeMappingKey(parser *yaml_parser_t,
	parentCtxt *decodeCtxt) (decodeCtxt, error) {

	ctxt, err := decodeNextValue(parser, parentCtxt)
	if err != nil {
		return ctxt, err
	}

	switch ctxt.state {
	case CTXT_STATE_DONE, CTXT_STATE_SCALAR:
		// Mapping complete or key decoded.
		return ctxt, nil

	default:
		return ctxt, decodeError(parser, "mapping lacks scalar key")
	}
}

func decodeMappingStart(parser *yaml_parser_t, event *yaml_event_t,
	parentCtxt *decodeCtxt) (decodeCtxt, error) {

	ctxt := decodeCtxt{state: CTXT_STATE_MAPPING}

	for {
		subCtxt, err := decodeMappingKey(parser, &ctxt)
		if err != nil || subCtxt.state == CTXT_STATE_DONE {
			return ctxt, err
		}
		key := stringValue(subCtxt.value)

		subCtxt, err = decodeNextValue(parser, &ctxt)
		if err != nil {
			return ctxt, err
		}
		if subCtxt.state == CTXT_STATE_DONE {
			return ctxt, decodeError(parser, "mapping lacks value")
		}
		mappingAdd(&ctxt, key, subCtxt.value)
	}

	return ctxt, nil
}

func decodeMappingEnd(parser *yaml_parser_t, event *yaml_event_t,
	parentCtxt *decodeCtxt) (decodeCtxt, error) {

	ctxt := decodeCtxt{state: CTXT_STATE_DONE}
	if parentCtxt.state != CTXT_STATE_MAPPING {
		return ctxt, decodeError(parser, "mapping end without start")
	}

	return ctxt, nil
}

func decodeScalar(parser *yaml_parser_t, event *yaml_event_t,
	parentCtxt *decodeCtxt) (decodeCtxt, error) {

	ctxt := decodeCtxt{state: CTXT_STATE_SCALAR}
	strVal := string(event.value)
	ctxt.value = genValue(strVal)

	return ctxt, nil
}

func decodeEvent(parser *yaml_parser_t,
	parentCtxt *decodeCtxt) (decodeCtxt, error) {

	event := yaml_event_t{}
	defer yaml_event_delete(&event)

	ctxt := decodeCtxt{state: CTXT_STATE_NONE}

	rc := yaml_parser_parse(parser, &event)
	if !rc {
		return ctxt, decodeError(parser, "%s", parser.problem)
	}

	fn := decodeDispatch[event.typ]
	if fn == nil {
		return ctxt, decodeError(parser, "Invalid event type: %s (%d)",
			constNames[event.typ], event.typ)
	}

	ctxt, err := fn(parser, &event, parentCtxt)
	return ctxt, err
}

func DecodeStream(b []byte, values map[string]interface{}) error {
	parser := yaml_parser_t{}

	initDecodeDispatch()

	yaml_parser_initialize(&parser)
	yaml_parser_set_input_string(&parser, b)

	// Decode YAML events until we get a valid mapping.
	for {
		ctxt := decodeCtxt{state: CTXT_STATE_NONE}
		subCtxt, err := decodeEvent(&parser, &ctxt)
		if err != nil {
			return err
		}

		switch subCtxt.state {
		case CTXT_STATE_MAPPING:
			// Copy mapping to input variable.
			for k, v := range subCtxt.value.(map[interface{}]interface{}) {
				values[k.(string)] = v
			}

		case CTXT_STATE_DONE:
			return nil

		default:
			// Unneeded metadata; proceed to next event.
			break
		}
	}
}

func SetFilename(filename string) {
	decodeFilename = filename
}
