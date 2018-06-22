// Copyright (c) 2018 Uber Technologies, Inc.
//
// Permission is hereby granted, free of charge, to any person obtaining a copy
// of this software and associated documentation files (the "Software"), to deal
// in the Software without restriction, including without limitation the rights
// to use, copy, modify, merge, publish, distribute, sublicense, and/or sell
// copies of the Software, and to permit persons to whom the Software is
// furnished to do so, subject to the following conditions:
//
// The above copyright notice and this permission notice shall be included in
// all copies or substantial portions of the Software.
//
// THE SOFTWARE IS PROVIDED "AS IS", WITHOUT WARRANTY OF ANY KIND, EXPRESS OR
// IMPLIED, INCLUDING BUT NOT LIMITED TO THE WARRANTIES OF MERCHANTABILITY,
// FITNESS FOR A PARTICULAR PURPOSE AND NONINFRINGEMENT. IN NO EVENT SHALL THE
// AUTHORS OR COPYRIGHT HOLDERS BE LIABLE FOR ANY CLAIM, DAMAGES OR OTHER
// LIABILITY, WHETHER IN AN ACTION OF CONTRACT, TORT OR OTHERWISE, ARISING FROM,
// OUT OF OR IN CONNECTION WITH THE SOFTWARE OR THE USE OR OTHER DEALINGS IN
// THE SOFTWARE.

package format

import (
	"fmt"
	"sort"
	"strings"
	"text/scanner"

	"github.com/emicklei/proto"
	"github.com/uber/prototool/internal/text"
)

type baseVisitor struct {
	*printer

	Failures []*text.Failure
}

func newBaseVisitor() *baseVisitor {
	return &baseVisitor{printer: newPrinter()}
}

func (v *baseVisitor) AddFailure(position scanner.Position, format string, args ...interface{}) {
	v.Failures = append(v.Failures, &text.Failure{
		Line:    position.Line,
		Column:  position.Column,
		Message: fmt.Sprintf(format, args...),
	})
}

func (v *baseVisitor) PWithInlineComment(inlineComment *proto.Comment, args ...interface{}) {
	if inlineComment == nil || len(inlineComment.Lines) == 0 {
		v.P(args...)
		return
	}
	// https://github.com/emicklei/proto/commit/5a91db7561a4dedab311f36304fcf0512343a9b1
	args = append(args, ` //`, cleanCommentLine(inlineComment.Lines[0]))
	v.P(args...)
	for _, line := range inlineComment.Lines[1:] {
		v.P(`//`, cleanCommentLine(line))
	}
}

func (v *baseVisitor) PComment(comment *proto.Comment) {
	if comment == nil || len(comment.Lines) == 0 {
		return
	}
	// https://github.com/emicklei/proto/commit/5a91db7561a4dedab311f36304fcf0512343a9b1
	// this is weird for now
	// we always want non-c-style after formatting
	for _, line := range comment.Lines {
		v.P(`//`, cleanCommentLine(line))
	}
}

func (v *baseVisitor) POptions(isFieldOption bool, options ...*proto.Option) {
	if len(options) == 0 {
		return
	}
	sort.Slice(options, func(i int, j int) bool { return options[i].Name < options[j].Name })
	prefix := "option "
	if isFieldOption {
		prefix = ""
	}
	for i, o := range options {
		suffix := ";"
		if isFieldOption {
			if len(options) > 1 && i != len(options)-1 {
				suffix = ","
			} else {
				suffix = ""
			}
		}
		v.PComment(o.Comment)
		// TODO: this is a good example of the reasoning for https://github.com/uber/prototool/issues/1
		if len(o.Constant.Array) == 0 && len(o.Constant.OrderedMap) == 0 {
			// SourceRepresentation() returns an empty string if the literal is empty
			// if empty, we do not want to print the key or empty value
			if source := o.Constant.SourceRepresentation(); source != "" {
				v.PWithInlineComment(o.InlineComment, prefix, o.Name, ` = `, source, suffix)
			}
		} else if len(o.Constant.Array) > 0 { // both Array and OrderedMap should not be set simultaneously, need more followup with emicklei/proto
			v.Failures = append(
				v.Failures,
				text.NewFailuref(o.Position, "INVALID_PROTOBUF", "top-level options should never be arrays, this should not compile with protoc"),
			)
		} else { // len(o.Constant.OrderedMap) > 0
			v.P(prefix, o.Name, ` = {`)
			v.In()
			for _, namedLiteral := range o.Constant.OrderedMap {
				v.pInnerLiteral(namedLiteral.Name, *namedLiteral.Literal, "")
			}
			v.Out()
			v.PWithInlineComment(o.InlineComment, `}`, suffix)
		}
	}
}

// should only be called by POptions
func (v *baseVisitor) pInnerLiteral(name string, literal proto.Literal, suffix string) {
	prefix := ""
	if name != "" {
		prefix = name + ": "
	}
	// TODO: this is a good example of the reasoning for https://github.com/uber/prototool/issues/1
	if len(literal.Array) == 0 && len(literal.OrderedMap) == 0 {
		// SourceRepresentation() returns an empty string if the literal is empty
		// if empty, we do not want to print the key or empty value
		if source := literal.SourceRepresentation(); source != "" {
			v.P(prefix, source, suffix)
		}
	} else if len(literal.Array) > 0 { // both Array and OrderedMap should not be set simultaneously, need more followup with emicklei/proto
		v.P(prefix, `[`)
		v.In()
		for i, iLiteral := range literal.Array {
			iSuffix := ""
			if len(literal.Array) > 1 && i != len(literal.Array)-1 {
				iSuffix = ","
			}
			v.pInnerLiteral("", *iLiteral, iSuffix)
		}
		v.Out()
		v.P(`]`, suffix)
	} else { // len(literal.OrderedMap) > 0
		v.P(prefix, `{`)
		v.In()
		for _, namedLiteral := range literal.OrderedMap {
			v.pInnerLiteral(namedLiteral.Name, *namedLiteral.Literal, "")
		}
		v.Out()
		v.P(`}`, suffix)
	}
}

func (v *baseVisitor) PField(prefix string, t string, field *proto.Field) {
	v.PComment(field.Comment)
	if len(field.Options) == 0 {
		v.PWithInlineComment(field.InlineComment, prefix, t, " ", field.Name, " = ", field.Sequence, ";")
		return
	}
	v.P(prefix, t, " ", field.Name, " = ", field.Sequence, " [")
	v.In()
	v.POptions(true, field.Options...)
	v.Out()
	v.PWithInlineComment(field.InlineComment, "];")
}

func cleanCommentLine(line string) string {
	// TODO: this is not great
	return strings.TrimLeft(line, "/")
}
