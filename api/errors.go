package api

import (
	"errors"
	"io"
	"os"
)

// FT_Err is the error code type matching FreeType's FT_Error.
type FT_Err int

const (
	FT_Err_Ok                        FT_Err = 0x00
	FT_Err_Cannot_Open_Resource      FT_Err = 0x01
	FT_Err_Unknown_File_Format       FT_Err = 0x02
	FT_Err_Invalid_File_Format       FT_Err = 0x03
	FT_Err_Invalid_Version           FT_Err = 0x04
	FT_Err_Lower_Module_Not_Found    FT_Err = 0x05
	FT_Err_Invalid_Argument          FT_Err = 0x06
	FT_Err_Invalid_Handle            FT_Err = 0x07
	FT_Err_Invalid_Glyph_Index       FT_Err = 0x08
	FT_Err_Invalid_Character_Code    FT_Err = 0x09
	FT_Err_Invalid_Glyph_Format      FT_Err = 0x0A
	FT_Err_Cannot_Render_Glyph       FT_Err = 0x0B
	FT_Err_Invalid_Outline           FT_Err = 0x0C
	FT_Err_Invalid_Composite         FT_Err = 0x0D
	FT_Err_Too_Many_Hints            FT_Err = 0x0E
	FT_Err_Invalid_Pixel_Size        FT_Err = 0x0F
	FT_Err_Invalid_Library_Handle    FT_Err = 0x10
	FT_Err_Invalid_Face_Handle       FT_Err = 0x11
	FT_Err_Invalid_Size_Handle       FT_Err = 0x12
	FT_Err_Invalid_Slot_Handle       FT_Err = 0x13
	FT_Err_Invalid_CharMap_Handle    FT_Err = 0x14
	FT_Err_Invalid_Cache_Handle      FT_Err = 0x15
	FT_Err_Invalid_Stream_Handle     FT_Err = 0x16
	FT_Err_Too_Many_Drivers          FT_Err = 0x17
	FT_Err_Too_Many_Extensions       FT_Err = 0x18
	FT_Err_Out_Of_Memory             FT_Err = 0x40
	FT_Err_Unlisted_Object           FT_Err = 0x41
	FT_Err_Nested_DEFS               FT_Err = 0x60
	FT_Err_Invalid_Table             FT_Err = 0x61
	FT_Err_Invalid_Horiz_Metrics     FT_Err = 0x62
	FT_Err_Invalid_CharMap_Format    FT_Err = 0x63
	FT_Err_Invalid_PPem              FT_Err = 0x64
	FT_Err_Invalid_Vert_Metrics      FT_Err = 0x65
	FT_Err_Could_Not_Find_Context    FT_Err = 0x66
	FT_Err_Invalid_Post_Table_Format FT_Err = 0x67
	FT_Err_Invalid_Post_Table        FT_Err = 0x68
	FT_Err_DEF_In_UTF8_Storage       FT_Err = 0x69
	FT_Err_Missing_Module            FT_Err = 0x6A
	FT_Err_Missing_Property          FT_Err = 0x6B
)

// CodedError is implemented by errors that carry a FreeType-compatible
// FT_Error code.
type CodedError interface {
	error
	FTErrorCode() FT_Err
}

// Error wraps an underlying Go error with a FreeType-compatible FT_Error code.
type Error struct {
	Code FT_Err
	Err  error
}

// NewError returns an error carrying a FreeType-compatible FT_Error code.
func NewError(code FT_Err, err error) error {
	if err == nil && code == FT_Err_Ok {
		return nil
	}
	return &Error{Code: code, Err: err}
}

func (e *Error) Error() string {
	if e == nil {
		return "<nil>"
	}
	if e.Err != nil {
		return e.Err.Error()
	}
	return "freetype error"
}

func (e *Error) Unwrap() error {
	if e == nil {
		return nil
	}
	return e.Err
}

func (e *Error) FTErrorCode() FT_Err {
	if e == nil {
		return FT_Err_Ok
	}
	return e.Code
}

// ErrorToCode maps a standard Go error to an FT_Err code.
func ErrorToCode(err error) FT_Err {
	if err == nil {
		return FT_Err_Ok
	}

	var coded CodedError
	if errors.As(err, &coded) {
		return coded.FTErrorCode()
	}

	if errors.Is(err, os.ErrNotExist) || errors.Is(err, os.ErrPermission) {
		return FT_Err_Cannot_Open_Resource
	}

	if errors.Is(err, io.EOF) || errors.Is(err, io.ErrUnexpectedEOF) {
		return FT_Err_Invalid_File_Format
	}

	// More specific mappings can be added as custom error types are defined.
	// For now, return a generic error if not matched.
	return FT_Err_Invalid_Argument
}
