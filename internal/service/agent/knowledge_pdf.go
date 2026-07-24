package agent

import (
	"bytes"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"regexp"
	"strings"
	"unicode"
	"unicode/utf16"
	"unicode/utf8"

	"github.com/ledongthuc/pdf"
)

const knowledgePDFExtractedByteLimit = knowledgeUploadMaxRunes*utf8.UTFMax + 1
const knowledgePDFCMapByteLimit = 4 << 20

var (
	errKnowledgePDFTextTooLarge        = errors.New("knowledge source exceeds 1000000 extracted characters")
	errKnowledgePDFUnsupportedEncoding = errors.New("PDF text encoding is not supported; export a searchable Unicode PDF or use OCR")

	pdfCMapCodeSpaceBlockPattern = regexp.MustCompile(`(?is)\d+\s+begincodespacerange(.*?)endcodespacerange`)
	pdfCMapBFCharBlockPattern    = regexp.MustCompile(`(?is)\d+\s+beginbfchar(.*?)endbfchar`)
	pdfCMapBFRangeBlockPattern   = regexp.MustCompile(`(?is)\d+\s+beginbfrange(.*?)endbfrange`)
	pdfCMapHexPattern            = regexp.MustCompile(`<([0-9a-fA-F\s]+)>`)
	pdfCMapPairPattern           = regexp.MustCompile(`(?is)<([0-9a-fA-F\s]+)>\s*<([0-9a-fA-F\s]+)>`)
	pdfCMapRangePattern          = regexp.MustCompile(`(?is)<([0-9a-fA-F\s]+)>\s*<([0-9a-fA-F\s]+)>\s*(<([0-9a-fA-F\s]+)>|\[(.*?)\])`)
)

// extractKnowledgePDFText extracts searchable text while handling the predefined
// CJK Unicode CMaps that are common in Chinese/Japanese/Korean generated PDFs.
func extractKnowledgePDFText(data []byte) (string, error) {
	reader, err := pdf.NewReader(bytes.NewReader(data), int64(len(data)))
	if err != nil {
		return "", errors.New("PDF file could not be parsed")
	}

	var result strings.Builder
	for pageNumber := 1; pageNumber <= reader.NumPage(); pageNumber++ {
		pageText, err := extractKnowledgePDFPageText(reader.Page(pageNumber))
		if err != nil {
			if errors.Is(err, errKnowledgePDFTextTooLarge) {
				return "", err
			}
			return "", errors.New("PDF text could not be extracted")
		}
		if result.Len()+len(pageText) > knowledgePDFExtractedByteLimit {
			return "", errKnowledgePDFTextTooLarge
		}
		result.WriteString(pageText)
	}

	content := strings.TrimSpace(result.String())
	if content == "" {
		return "", errors.New("PDF contains no extractable text; scanned PDFs require OCR")
	}
	if err := validateExtractedPDFText(content); err != nil {
		return "", err
	}
	return content, nil
}

// extractKnowledgePDFPageText mirrors the dependency's text-operator walk but
// selects the decoder from each page's actual font resource. The dependency's
// default decoder does not recognize predefined Uni*-UCS2 CMaps.
func extractKnowledgePDFPageText(page pdf.Page) (result string, err error) {
	defer func() {
		if recovered := recover(); recovered != nil {
			result = ""
			err = fmt.Errorf("%v", recovered)
		}
	}()
	if page.V.IsNull() || page.V.Key("Contents").Kind() == pdf.Null {
		return "", nil
	}

	encoders := make(map[string]pdf.TextEncoding, len(page.Fonts()))
	for _, name := range page.Fonts() {
		encoders[name] = knowledgePDFTextEncoder(page.Font(name))
	}

	var text strings.Builder
	var encoder pdf.TextEncoding = passthroughPDFTextEncoder{}
	writeDecoded := func(raw string) {
		if text.Len() >= knowledgePDFExtractedByteLimit {
			return
		}
		decoded := encoder.Decode(raw)
		if text.Len()+len(decoded) > knowledgePDFExtractedByteLimit {
			decoded = decoded[:knowledgePDFExtractedByteLimit-text.Len()]
		}
		text.WriteString(decoded)
	}

	pdf.Interpret(page.V.Key("Contents"), func(stack *pdf.Stack, operator string) {
		count := stack.Len()
		arguments := make([]pdf.Value, count)
		for index := count - 1; index >= 0; index-- {
			arguments[index] = stack.Pop()
		}
		switch operator {
		case "BT":
			text.WriteByte('\n')
		case "T*":
			text.WriteByte('\n')
		case "Tf":
			if len(arguments) != 2 {
				panic("invalid PDF Tf operator")
			}
			encoder = encoders[arguments[0].Name()]
			if encoder == nil {
				encoder = passthroughPDFTextEncoder{}
			}
		case "\"":
			if len(arguments) != 3 {
				panic("invalid PDF quote operator")
			}
			text.WriteByte('\n')
			writeDecoded(arguments[2].RawString())
		case "'":
			if len(arguments) != 1 {
				panic("invalid PDF quote operator")
			}
			text.WriteByte('\n')
			writeDecoded(arguments[0].RawString())
		case "Tj":
			if len(arguments) != 1 {
				panic("invalid PDF Tj operator")
			}
			writeDecoded(arguments[0].RawString())
		case "TJ":
			if len(arguments) != 1 {
				panic("invalid PDF TJ operator")
			}
			for index := 0; index < arguments[0].Len(); index++ {
				value := arguments[0].Index(index)
				if value.Kind() == pdf.String {
					writeDecoded(value.RawString())
				}
			}
		}
	})
	if text.Len() >= knowledgePDFExtractedByteLimit {
		return "", errKnowledgePDFTextTooLarge
	}
	return text.String(), nil
}

func knowledgePDFTextEncoder(font pdf.Font) pdf.TextEncoding {
	if toUnicode := font.V.Key("ToUnicode"); toUnicode.Kind() == pdf.Stream {
		if encoder := newToUnicodePDFTextEncoder(toUnicode); encoder != nil {
			return encoder
		}
		return unsupportedPDFTextEncoder{}
	}

	encoding := font.V.Key("Encoding").Name()
	switch encoding {
	case "UniGB-UCS2-H", "UniGB-UCS2-V",
		"UniCNS-UCS2-H", "UniCNS-UCS2-V",
		"UniJIS-UCS2-H", "UniJIS-UCS2-V",
		"UniJIS-UTF16-H", "UniJIS-UTF16-V",
		"UniKS-UCS2-H", "UniKS-UCS2-V":
		return utf16BEPDFTextEncoder{}
	case "Identity-H", "Identity-V":
		return unsupportedPDFTextEncoder{}
	case "":
		if font.V.Key("Subtype").Name() == "Type0" {
			return unsupportedPDFTextEncoder{}
		}
	default:
		if font.V.Key("Subtype").Name() == "Type0" {
			return unsupportedPDFTextEncoder{}
		}
	}
	return font.Encoder()
}

type pdfCodeSpaceRange struct {
	low  string
	high string
}

type pdfUnicodeRange struct {
	low          string
	high         string
	destinations []string
	sequential   bool
}

type toUnicodePDFTextEncoder struct {
	codeSpaces [4][]pdfCodeSpaceRange
	characters map[string]string
	ranges     []pdfUnicodeRange
}

// newToUnicodePDFTextEncoder parses an embedded ToUnicode CMap. The dependency
// skips this map whenever a simple font also has a custom Encoding dictionary,
// which is common for macOS-generated Type 3 CJK PDFs.
func newToUnicodePDFTextEncoder(stream pdf.Value) (encoder pdf.TextEncoding) {
	defer func() {
		if recover() != nil {
			encoder = nil
		}
	}()

	reader := stream.Reader()
	defer reader.Close()
	data, err := io.ReadAll(io.LimitReader(reader, knowledgePDFCMapByteLimit+1))
	if err != nil || len(data) > knowledgePDFCMapByteLimit {
		return nil
	}

	result := &toUnicodePDFTextEncoder{characters: make(map[string]string)}
	for _, block := range pdfCMapCodeSpaceBlockPattern.FindAllSubmatch(data, -1) {
		for _, match := range pdfCMapPairPattern.FindAllSubmatch(block[1], -1) {
			low, lowOK := decodePDFHex(match[1])
			high, highOK := decodePDFHex(match[2])
			if !lowOK || !highOK || len(low) == 0 || len(low) != len(high) || len(low) > len(result.codeSpaces) {
				return nil
			}
			result.codeSpaces[len(low)-1] = append(result.codeSpaces[len(low)-1], pdfCodeSpaceRange{low: low, high: high})
		}
	}
	for _, block := range pdfCMapBFCharBlockPattern.FindAllSubmatch(data, -1) {
		for _, match := range pdfCMapPairPattern.FindAllSubmatch(block[1], -1) {
			original, originalOK := decodePDFHex(match[1])
			replacement, replacementOK := decodePDFHex(match[2])
			if !originalOK || !replacementOK || original == "" || replacement == "" {
				return nil
			}
			result.characters[original] = replacement
		}
	}
	for _, block := range pdfCMapBFRangeBlockPattern.FindAllSubmatch(data, -1) {
		for _, match := range pdfCMapRangePattern.FindAllSubmatch(block[1], -1) {
			low, lowOK := decodePDFHex(match[1])
			high, highOK := decodePDFHex(match[2])
			if !lowOK || !highOK || low == "" || len(low) != len(high) {
				return nil
			}
			mappedRange := pdfUnicodeRange{low: low, high: high}
			if len(match[4]) > 0 {
				destination, ok := decodePDFHex(match[4])
				if !ok || destination == "" {
					return nil
				}
				mappedRange.destinations = []string{destination}
				mappedRange.sequential = true
			} else {
				for _, destinationMatch := range pdfCMapHexPattern.FindAllSubmatch(match[5], -1) {
					destination, ok := decodePDFHex(destinationMatch[1])
					if !ok || destination == "" {
						return nil
					}
					mappedRange.destinations = append(mappedRange.destinations, destination)
				}
				if len(mappedRange.destinations) == 0 {
					return nil
				}
			}
			result.ranges = append(result.ranges, mappedRange)
		}
	}
	if len(result.characters) == 0 && len(result.ranges) == 0 {
		return nil
	}
	return result
}

func decodePDFHex(encoded []byte) (string, bool) {
	compact := strings.Join(strings.Fields(string(encoded)), "")
	if compact == "" {
		return "", false
	}
	if len(compact)%2 != 0 {
		compact += "0"
	}
	decoded, err := hex.DecodeString(compact)
	return string(decoded), err == nil
}

func (e *toUnicodePDFTextEncoder) Decode(raw string) string {
	var result strings.Builder
	for len(raw) > 0 {
		codeLength := e.codeLength(raw)
		if codeLength == 0 {
			result.WriteRune(utf8.RuneError)
			raw = raw[1:]
			continue
		}

		code := raw[:codeLength]
		raw = raw[codeLength:]
		if replacement, ok := e.characters[code]; ok {
			result.WriteString(decodeUTF16BE(replacement))
			continue
		}
		if replacement, ok := e.rangeReplacement(code); ok {
			result.WriteString(decodeUTF16BE(replacement))
			continue
		}
		result.WriteRune(utf8.RuneError)
	}
	return result.String()
}

func (e *toUnicodePDFTextEncoder) codeLength(raw string) int {
	for length := 1; length <= len(e.codeSpaces) && length <= len(raw); length++ {
		for _, codeSpace := range e.codeSpaces[length-1] {
			if codeSpace.low <= raw[:length] && raw[:length] <= codeSpace.high {
				return length
			}
		}
	}
	return 0
}

func (e *toUnicodePDFTextEncoder) rangeReplacement(code string) (string, bool) {
	for _, mappedRange := range e.ranges {
		if len(mappedRange.low) != len(code) || code < mappedRange.low || code > mappedRange.high {
			continue
		}
		offset := bigEndianCodeValue(code) - bigEndianCodeValue(mappedRange.low)
		if mappedRange.sequential {
			return incrementBigEndianBytes(mappedRange.destinations[0], offset), true
		}
		if offset > uint64(len(mappedRange.destinations)-1) {
			return "", false
		}
		if mappedRange.destinations[offset] == "" {
			return "", false
		}
		return mappedRange.destinations[offset], true
	}
	return "", false
}

func bigEndianCodeValue(code string) uint64 {
	var value uint64
	for index := 0; index < len(code); index++ {
		value = value<<8 | uint64(code[index])
	}
	return value
}

func incrementBigEndianBytes(value string, increment uint64) string {
	bytes := []byte(value)
	for index := len(bytes) - 1; index >= 0 && increment > 0; index-- {
		total := uint64(bytes[index]) + increment
		bytes[index] = byte(total)
		increment = total >> 8
	}
	return string(bytes)
}

type passthroughPDFTextEncoder struct{}

func (passthroughPDFTextEncoder) Decode(raw string) string {
	return raw
}

type unsupportedPDFTextEncoder struct{}

func (unsupportedPDFTextEncoder) Decode(raw string) string {
	if raw == "" {
		return ""
	}
	return string(utf8.RuneError)
}

type utf16BEPDFTextEncoder struct{}

func (utf16BEPDFTextEncoder) Decode(raw string) string {
	return decodeUTF16BE(raw)
}

func decodeUTF16BE(raw string) string {
	if len(raw)%2 != 0 {
		return string(utf8.RuneError)
	}
	codeUnits := make([]uint16, 0, len(raw)/2)
	for index := 0; index < len(raw); index += 2 {
		codeUnits = append(codeUnits, uint16(raw[index])<<8|uint16(raw[index+1]))
	}
	return string(utf16.Decode(codeUnits))
}

func validateExtractedPDFText(content string) error {
	if !utf8.ValidString(content) {
		return errKnowledgePDFUnsupportedEncoding
	}
	for _, character := range content {
		if character == utf8.RuneError {
			return errKnowledgePDFUnsupportedEncoding
		}
		if unicode.IsControl(character) && character != '\n' && character != '\r' && character != '\t' {
			return errKnowledgePDFUnsupportedEncoding
		}
	}
	return nil
}
