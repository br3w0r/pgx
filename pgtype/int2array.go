package pgtype

import (
	"bytes"
	"fmt"
	"io"

	"github.com/jackc/pgx/pgio"
)

type Int2Array struct {
	Elements   []Int2
	Dimensions []ArrayDimension
	Status     Status
}

func (dst *Int2Array) ConvertFrom(src interface{}) error {
	switch value := src.(type) {
	case Int2Array:
		*dst = value

	case []int16:
		if value == nil {
			*dst = Int2Array{Status: Null}
		} else if len(value) == 0 {
			*dst = Int2Array{Status: Present}
		} else {
			elements := make([]Int2, len(value))
			for i := range value {
				if err := elements[i].ConvertFrom(value[i]); err != nil {
					return err
				}
			}
			*dst = Int2Array{
				Elements:   elements,
				Dimensions: []ArrayDimension{{Length: int32(len(elements)), LowerBound: 1}},
				Status:     Present,
			}
		}

	case []uint16:
		if value == nil {
			*dst = Int2Array{Status: Null}
		} else if len(value) == 0 {
			*dst = Int2Array{Status: Present}
		} else {
			elements := make([]Int2, len(value))
			for i := range value {
				if err := elements[i].ConvertFrom(value[i]); err != nil {
					return err
				}
			}
			*dst = Int2Array{
				Elements:   elements,
				Dimensions: []ArrayDimension{{Length: int32(len(elements)), LowerBound: 1}},
				Status:     Present,
			}
		}

	default:
		if originalSrc, ok := underlyingSliceType(src); ok {
			return dst.ConvertFrom(originalSrc)
		}
		return fmt.Errorf("cannot convert %v to Int2", value)
	}

	return nil
}

func (src *Int2Array) AssignTo(dst interface{}) error {
	switch v := dst.(type) {

	case *[]int16:
		if src.Status == Present {
			*v = make([]int16, len(src.Elements))
			for i := range src.Elements {
				if err := src.Elements[i].AssignTo(&((*v)[i])); err != nil {
					return err
				}
			}
		} else {
			*v = nil
		}

	case *[]uint16:
		if src.Status == Present {
			*v = make([]uint16, len(src.Elements))
			for i := range src.Elements {
				if err := src.Elements[i].AssignTo(&((*v)[i])); err != nil {
					return err
				}
			}
		} else {
			*v = nil
		}

	default:
		if originalDst, ok := underlyingPtrSliceType(dst); ok {
			return src.AssignTo(originalDst)
		}
		return fmt.Errorf("cannot decode %v into %T", src, dst)
	}

	return nil
}

func (dst *Int2Array) DecodeText(r io.Reader) error {
	size, err := pgio.ReadInt32(r)
	if err != nil {
		return err
	}

	if size == -1 {
		*dst = Int2Array{Status: Null}
		return nil
	}

	buf := make([]byte, int(size))
	_, err = io.ReadFull(r, buf)
	if err != nil {
		return err
	}

	uta, err := ParseUntypedTextArray(string(buf))
	if err != nil {
		return err
	}

	textElementReader := NewTextElementReader(r)
	var elements []Int2

	if len(uta.Elements) > 0 {
		elements = make([]Int2, len(uta.Elements))

		for i, s := range uta.Elements {
			var elem Int2
			textElementReader.Reset(s)
			err = elem.DecodeText(textElementReader)
			if err != nil {
				return err
			}

			elements[i] = elem
		}
	}

	*dst = Int2Array{Elements: elements, Dimensions: uta.Dimensions, Status: Present}

	return nil
}

func (dst *Int2Array) DecodeBinary(r io.Reader) error {
	size, err := pgio.ReadInt32(r)
	if err != nil {
		return err
	}

	if size == -1 {
		*dst = Int2Array{Status: Null}
		return nil
	}

	var arrayHeader ArrayHeader
	err = arrayHeader.DecodeBinary(r)
	if err != nil {
		return err
	}

	if len(arrayHeader.Dimensions) == 0 {
		*dst = Int2Array{Dimensions: arrayHeader.Dimensions, Status: Present}
		return nil
	}

	elementCount := arrayHeader.Dimensions[0].Length
	for _, d := range arrayHeader.Dimensions[1:] {
		elementCount *= d.Length
	}

	elements := make([]Int2, elementCount)

	for i := range elements {
		err = elements[i].DecodeBinary(r)
		if err != nil {
			return err
		}
	}

	*dst = Int2Array{Elements: elements, Dimensions: arrayHeader.Dimensions, Status: Present}
	return nil
}

func (src *Int2Array) EncodeText(w io.Writer) error {
	if done, err := encodeNotPresent(w, src.Status); done {
		return err
	}

	if len(src.Dimensions) == 0 {
		_, err := pgio.WriteInt32(w, 2)
		if err != nil {
			return err
		}

		_, err = w.Write([]byte("{}"))
		return err
	}

	buf := &bytes.Buffer{}

	err := EncodeTextArrayDimensions(buf, src.Dimensions)
	if err != nil {
		return err
	}

	// dimElemCounts is the multiples of elements that each array lies on. For
	// example, a single dimension array of length 4 would have a dimElemCounts of
	// [4]. A multi-dimensional array of lengths [3,5,2] would have a
	// dimElemCounts of [30,10,2]. This is used to simplify when to render a '{'
	// or '}'.
	dimElemCounts := make([]int, len(src.Dimensions))
	dimElemCounts[len(src.Dimensions)-1] = int(src.Dimensions[len(src.Dimensions)-1].Length)
	for i := len(src.Dimensions) - 2; i > -1; i-- {
		dimElemCounts[i] = int(src.Dimensions[i].Length) * dimElemCounts[i+1]
	}

	textElementWriter := NewTextElementWriter(buf)

	for i, elem := range src.Elements {
		if i > 0 {
			err = pgio.WriteByte(buf, ',')
			if err != nil {
				return err
			}
		}

		for _, dec := range dimElemCounts {
			if i%dec == 0 {
				err = pgio.WriteByte(buf, '{')
				if err != nil {
					return err
				}
			}
		}

		textElementWriter.Reset()
		err = elem.EncodeText(textElementWriter)
		if err != nil {
			return err
		}

		for _, dec := range dimElemCounts {
			if (i+1)%dec == 0 {
				err = pgio.WriteByte(buf, '}')
				if err != nil {
					return err
				}
			}
		}
	}

	_, err = pgio.WriteInt32(w, int32(buf.Len()))
	if err != nil {
		return err
	}

	_, err = buf.WriteTo(w)
	return err
}

func (src *Int2Array) EncodeBinary(w io.Writer) error {
	if done, err := encodeNotPresent(w, src.Status); done {
		return err
	}

	var arrayHeader ArrayHeader

	// TODO - consider how to avoid having to buffer array before writing length -
	// or how not pay allocations for the byte order conversions.
	elemBuf := &bytes.Buffer{}

	for i := range src.Elements {
		err := src.Elements[i].EncodeBinary(elemBuf)
		if err != nil {
			return err
		}
		if src.Elements[i].Status == Null {
			arrayHeader.ContainsNull = true
		}
	}

	arrayHeader.ElementOID = Int2OID
	arrayHeader.Dimensions = src.Dimensions

	// TODO - consider how to avoid having to buffer array before writing length -
	// or how not pay allocations for the byte order conversions.
	headerBuf := &bytes.Buffer{}
	err := arrayHeader.EncodeBinary(headerBuf)
	if err != nil {
		return err
	}

	_, err = pgio.WriteInt32(w, int32(headerBuf.Len()+elemBuf.Len()))
	if err != nil {
		return err
	}

	_, err = headerBuf.WriteTo(w)
	if err != nil {
		return err
	}

	_, err = elemBuf.WriteTo(w)
	if err != nil {
		return err
	}

	return err
}
