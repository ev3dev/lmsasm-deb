// Copyright 2009 The Go Authors. All rights reserved.
// Copyright 2016 David Lechner <david@lechnology.com>
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

// Package scanner provides a scanner and tokenizer for UTF-8-encoded text.
// It takes an io.Reader providing the source, which then can be tokenized
// through repeated calls to the Scan function. For compatibility with
// existing tools, the NUL character is not allowed. If the first character
// in the source is a UTF-8 encoded byte order mark (BOM), it is discarded.
package scanner

import (
	"bytes"
	"fmt"
	"github.com/ev3dev/lmsasm/token"
	"io"
	"os"
	"unicode/utf8"
)

const whitespace = 1<<'\t' | 1<<'\n' | 1<<'\r' | 1<<' '

const bufLen = 1024 // at least utf8.UTFMax

// A Scanner implements reading of Unicode characters and tokens from an io.Reader.
type Scanner struct {
	// Input
	src io.Reader

	// Source buffer
	srcBuf [bufLen + 1]byte // +1 for sentinel for common case of s.next()
	srcPos int              // reading position (srcBuf index)
	srcEnd int              // source end (srcBuf index)

	// Source position
	srcBufOffset int // byte offset of srcBuf[0] in source
	line         int // line count
	column       int // character count
	lastLineLen  int // length of last line in characters (for correct column reporting)
	lastCharLen  int // length of last character in bytes

	// Token text buffer
	// Typically, token text is stored completely in srcBuf, but in general
	// the token text's head may be buffered in tokBuf while the token text's
	// tail is stored in srcBuf.
	tokBuf bytes.Buffer // token text head that is not in srcBuf anymore
	tokPos int          // token text tail position (srcBuf index); valid if >= 0
	tokEnd int          // token text tail end (srcBuf index)

	// One character look-ahead
	ch rune // character before current srcPos

	// Error is called for each error encountered. If no Error
	// function is set, the error is reported to os.Stderr.
	Error func(s *Scanner, msg string)

	// ErrorCount is incremented by one for each error encountered.
	ErrorCount int

	// Start position of most recently scanned token; set by Scan.
	// Calling Init or Next invalidates the position (Line == 0).
	// The Filename field is always left untouched by the Scanner.
	// If an error is reported (via Error) and Position is invalid,
	// the scanner is not inside a token. Call Pos to obtain an error
	// position in that case.
	Position token.Position
}

// Init initializes a Scanner with a new source and returns s.
// Error is set to nil, ErrorCount is set to 0.
func (s *Scanner) Init(src io.Reader, filename string) *Scanner {
	s.src = src

	// initialize source buffer
	// (the first call to next() will fill it by calling src.Read)
	s.srcBuf[0] = utf8.RuneSelf // sentinel
	s.srcPos = 0
	s.srcEnd = 0

	// initialize source position
	s.srcBufOffset = 0
	s.line = 1
	s.column = 0
	s.lastLineLen = 0
	s.lastCharLen = 0

	// initialize token text buffer
	// (required for first call to next()).
	s.tokPos = -1

	// initialize one character look-ahead
	s.ch = -2 // no char read yet, not EOF

	// initialize public fields
	s.Error = nil
	s.ErrorCount = 0
	s.Position.Line = 0 // invalidate token position
	s.Position.Filename = filename

	return s
}

// next reads and returns the next Unicode character. It is designed such
// that only a minimal amount of work needs to be done in the common ASCII
// case (one test to check for both ASCII and end-of-buffer, and one test
// to check for newlines).
func (s *Scanner) next() rune {
	ch, width := rune(s.srcBuf[s.srcPos]), 1

	if ch >= utf8.RuneSelf {
		// uncommon case: not ASCII or not enough bytes
		for s.srcPos+utf8.UTFMax > s.srcEnd && !utf8.FullRune(s.srcBuf[s.srcPos:s.srcEnd]) {
			// not enough bytes: read some more, but first
			// save away token text if any
			if s.tokPos >= 0 {
				s.tokBuf.Write(s.srcBuf[s.tokPos:s.srcPos])
				s.tokPos = 0
				// s.tokEnd is set by Scan()
			}
			// move unread bytes to beginning of buffer
			copy(s.srcBuf[0:], s.srcBuf[s.srcPos:s.srcEnd])
			s.srcBufOffset += s.srcPos
			// read more bytes
			// (an io.Reader must return io.EOF when it reaches
			// the end of what it is reading - simply returning
			// n == 0 will make this loop retry forever; but the
			// error is in the reader implementation in that case)
			i := s.srcEnd - s.srcPos
			n, err := s.src.Read(s.srcBuf[i:bufLen])
			s.srcPos = 0
			s.srcEnd = i + n
			s.srcBuf[s.srcEnd] = utf8.RuneSelf // sentinel
			if err != nil {
				if err != io.EOF {
					s.error(err.Error())
				}
				if s.srcEnd == 0 {
					if s.lastCharLen > 0 {
						// previous character was not EOF
						s.column++
					}
					s.lastCharLen = 0
					return -1
				}
				// If err == EOF, we won't be getting more
				// bytes; break to avoid infinite loop. If
				// err is something else, we don't know if
				// we can get more bytes; thus also break.
				break
			}
		}
		// at least one byte
		ch = rune(s.srcBuf[s.srcPos])
		if ch >= utf8.RuneSelf {
			// uncommon case: not ASCII
			ch, width = utf8.DecodeRune(s.srcBuf[s.srcPos:s.srcEnd])
			if ch == utf8.RuneError && width == 1 {
				// advance for correct error position
				s.srcPos += width
				s.lastCharLen = width
				s.column++
				s.error("illegal UTF-8 encoding")
				return ch
			}
		}
	}

	// advance
	s.srcPos += width
	s.lastCharLen = width
	s.column++

	// special situations
	switch ch {
	case 0:
		// for compatibility with other tools
		s.error("illegal character NUL")
	case '\n':
		s.line++
		s.lastLineLen = s.column
		s.column = 0
	}

	return ch
}

// Peek returns the next Unicode character in the source without advancing
// the scanner. It returns -1 if the scanner's position is at the last
// character of the source.
func (s *Scanner) Peek() rune {
	if s.ch == -2 {
		// this code is only run for the very first character
		s.ch = s.next()
		if s.ch == '\uFEFF' {
			s.ch = s.next() // ignore BOM
		}
	}
	return s.ch
}

func (s *Scanner) error(msg string) {
	s.ErrorCount++
	if s.Error != nil {
		s.Error(s, msg)
		return
	}
	pos := s.Position
	if !pos.IsValid() {
		pos = s.Pos()
	}
	fmt.Fprintf(os.Stderr, "%s: %s\n", pos, msg)
}

func (s *Scanner) isIdentRune(ch rune, i int) bool {
	return ch == '_' || ('a' <= ch && ch <= 'z') || ('A' <= ch && ch <= 'Z') || (isDecimal(ch) && i > 0)
}

func (s *Scanner) scanIdentifier() rune {
	// we know the zero'th rune is OK; start scanning at the next one
	ch := s.next()
	for i := 1; s.isIdentRune(ch, i); i++ {
		ch = s.next()
	}
	return ch
}

func digitVal(ch rune) int {
	switch {
	case '0' <= ch && ch <= '9':
		return int(ch - '0')
	case 'a' <= ch && ch <= 'f':
		return int(ch - 'a' + 10)
	case 'A' <= ch && ch <= 'F':
		return int(ch - 'A' + 10)
	}
	return 16 // larger than any legal digit val
}

func isDecimal(ch rune) bool { return '0' <= ch && ch <= '9' }

func (s *Scanner) scanMantissa(ch rune) rune {
	for isDecimal(ch) {
		ch = s.next()
	}
	return ch
}

func (s *Scanner) scanFraction(ch rune) rune {
	if ch == '.' {
		ch = s.scanMantissa(s.next())
	}
	return ch
}

func (s *Scanner) scanExponent(ch rune) rune {
	if ch == 'e' || ch == 'E' {
		ch = s.next()
		if ch == '-' || ch == '+' {
			ch = s.next()
		}
		ch = s.scanMantissa(ch)
	}
	return ch
}

func (s *Scanner) scanNumber(ch rune) (token.Token, rune) {
	// isDecimal(ch)
	if ch == '0' {
		// int or float
		ch = s.next()
		if ch == 'x' || ch == 'X' {
			// hexadecimal int
			ch = s.next()
			hasMantissa := false
			for digitVal(ch) < 16 {
				ch = s.next()
				hasMantissa = true
			}
			if !hasMantissa {
				s.error("illegal hexadecimal number")
			}
		} else {
			// octal int or float
			if ch == 'F' {
				ch = s.next()
				return token.FLOAT, ch
			}
			has8or9 := false
			for isDecimal(ch) {
				if ch > '7' {
					has8or9 = true
				}
				ch = s.next()
			}
			if ch == 'F' {
				ch = s.next()
				return token.FLOAT, ch
			}
			if ch == '.' || ch == 'e' || ch == 'E' {
				// float
				ch = s.scanFraction(ch)
				ch = s.scanExponent(ch)
				if ch == 'F' {
					ch = s.next()
				} else {
					s.error("float is missing 'F' suffix")
				}
				return token.FLOAT, ch
			}
			// octal int
			if has8or9 {
				s.error("illegal octal number")
			}
		}
		return token.INT, ch
	}
	// decimal int or float
	ch = s.scanMantissa(ch)
	if ch == 'F' {
		ch = s.next()
		return token.FLOAT, ch
	}
	if ch == '.' || ch == 'e' || ch == 'E' {
		// float
		ch = s.scanFraction(ch)
		ch = s.scanExponent(ch)
		if ch == 'F' {
			ch = s.next()
		} else {
			s.error("float is missing 'F' suffix")
		}
		return token.FLOAT, ch
	}
	return token.INT, ch
}

func (s *Scanner) scanDigits(ch rune, base, n int) rune {
	for n > 0 && digitVal(ch) < base {
		ch = s.next()
		n--
	}
	if n > 0 {
		s.error("illegal char escape")
	}
	return ch
}

func (s *Scanner) scanEscape(quote rune) rune {
	ch := s.next() // read character after '/'
	switch ch {
	// case 'a', 'b', 'f', 'n', 'r', 't', 'v', '\\', quote:
	case 'n', 'r', 't', 'q', '\\':
		// nothing to do
		ch = s.next()
	// case '0', '1', '2', '3', '4', '5', '6', '7':
	// 	ch = s.scanDigits(ch, 8, 3)
	// case 'x':
	// 	ch = s.scanDigits(s.next(), 16, 2)
	// case 'u':
	// 	ch = s.scanDigits(s.next(), 16, 4)
	// case 'U':
	// 	ch = s.scanDigits(s.next(), 16, 8)
	default:
		s.error("illegal char escape")
	}
	return ch
}

func (s *Scanner) scanString(quote rune) {
	ch := s.next() // read character after quote
	for ch != quote {
		if ch == '\n' || ch < 0 {
			s.error("literal not terminated")
			return
		}
		if ch == '\\' {
			ch = s.scanEscape(quote)
		} else {
			ch = s.next()
		}
	}
	return
}

func (s *Scanner) scanComment(ch rune) rune {
	// ch == '/' || ch == '*'
	if ch == '/' {
		// line comment
		ch = s.next() // read character after "//"
		for ch != '\n' && ch >= 0 {
			ch = s.next()
		}
		return ch
	}

	// general comment
	ch = s.next() // read character after "/*"
	for {
		if ch < 0 {
			s.error("comment not terminated")
			break
		}
		ch0 := ch
		ch = s.next()
		if ch0 == '*' && ch == '/' {
			ch = s.next()
			break
		}
	}
	return ch
}

// Scan reads the next token or Unicode character from source and returns it.
// It returns EOF at the end of the source. It reports scanner errors (read and
// token errors) by calling s.Error, if not nil; otherwise it prints an error
// message to os.Stderr.
func (s *Scanner) Scan() token.Token {
	ch := s.Peek()

	// reset token text position
	s.tokPos = -1
	s.Position.Line = 0

redo:
	// skip white space
	for whitespace&(1<<uint(ch)) != 0 {
		ch = s.next()
	}

	// start collecting token text
	s.tokBuf.Reset()
	s.tokPos = s.srcPos - s.lastCharLen

	// set token position
	// (this is a slightly optimized version of the code in Pos())
	s.Position.Offset = s.srcBufOffset + s.tokPos
	if s.column > 0 {
		// common case: last character was not a '\n'
		s.Position.Line = s.line
		s.Position.Column = s.column
	} else {
		// last character was a '\n'
		// (we cannot be at the beginning of the source
		// since we have called next() at least once)
		s.Position.Line = s.line - 1
		s.Position.Column = s.lastLineLen
	}

	// determine token value
	tok := token.ILLEGAL
	switch {
	case s.isIdentRune(ch, 0):
		tok = token.IDENT
		ch = s.scanIdentifier()
	case isDecimal(ch):
		tok, ch = s.scanNumber(ch)
	default:
		switch ch {
		case -1:
			tok = token.EOF
		case '\'':
			s.scanString('\'')
			tok = token.STRING
			ch = s.next()
		case '.':
			ch = s.next()
			if isDecimal(ch) {
				tok = token.FLOAT
				ch = s.scanMantissa(ch)
				ch = s.scanExponent(ch)
				if ch == 'F' {
					ch = s.next()
				} else {
					s.error("float is missing 'F' suffix")
				}
			}
		case '+':
			tok = token.ADD
			ch = s.next()
		case '-':
			// negitive numbers should already be handled
			tok = token.SUB
			ch = s.next()
		case '*':
			tok = token.MUL
			ch = s.next()
		case '/':
			ch = s.next()
			if ch == '/' || ch == '*' {
				s.tokPos = -1 // don't collect token text
				ch = s.scanComment(ch)
				goto redo
			} else {
				tok = token.QUO
			}
		case '@':
			tok = token.AT
			ch = s.next()
		case '(':
			tok = token.LPAREN
			ch = s.next()
		case ')':
			tok = token.RPAREN
			ch = s.next()
		case '{':
			tok = token.LBRACE
			ch = s.next()
		case '}':
			tok = token.RBRACE
			ch = s.next()
		case ',':
			tok = token.COMMA
			ch = s.next()
		case ':':
			tok = token.COLON
			ch = s.next()
		default:
			ch = s.next()
		}
	}

	// end of token text
	s.tokEnd = s.srcPos - s.lastCharLen

	s.ch = ch
	return tok
}

// Pos returns the position of the character immediately after
// the character or token returned by the last call to Next or Scan.
func (s *Scanner) Pos() (pos token.Position) {
	pos.Filename = s.Position.Filename
	pos.Offset = s.srcBufOffset + s.srcPos - s.lastCharLen
	switch {
	case s.column > 0:
		// common case: last character was not a '\n'
		pos.Line = s.line
		pos.Column = s.column
	case s.lastLineLen > 0:
		// last character was a '\n'
		pos.Line = s.line - 1
		pos.Column = s.lastLineLen
	default:
		// at the beginning of the source
		pos.Line = 1
		pos.Column = 1
	}
	return
}

// TokenText returns the string corresponding to the most recently scanned token.
// Valid after calling Scan().
func (s *Scanner) TokenText() string {
	if s.tokPos < 0 {
		// no token text
		return ""
	}

	if s.tokEnd < 0 {
		// if EOF was reached, s.tokEnd is set to -1 (s.srcPos == 0)
		s.tokEnd = s.tokPos
	}

	if s.tokBuf.Len() == 0 {
		// common case: the entire token text is still in srcBuf
		return string(s.srcBuf[s.tokPos:s.tokEnd])
	}

	// part of the token text was saved in tokBuf: save the rest in
	// tokBuf as well and return its content
	s.tokBuf.Write(s.srcBuf[s.tokPos:s.tokEnd])
	s.tokPos = s.tokEnd // ensure idempotency of TokenText() call
	return s.tokBuf.String()
}
