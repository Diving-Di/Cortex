package store

import (
	"strconv"
	"strings"
)

type knowledgeDocumentScanner interface {
	Scan(dest ...any) error
}

func vectorLiteral(values []float32) string {
	var output strings.Builder
	output.WriteByte('[')
	for index, value := range values {
		if index > 0 {
			output.WriteByte(',')
		}
		output.WriteString(strconv.FormatFloat(float64(value), 'g', -1, 32))
	}
	output.WriteByte(']')
	return output.String()
}
