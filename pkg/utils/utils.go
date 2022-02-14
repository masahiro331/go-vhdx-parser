package utils

import (
	"bytes"
	"encoding/binary"
	"golang.org/x/xerrors"
	"io"
)

func BinaryRead(r io.Reader, o binary.ByteOrder, v interface{}, align int) error {
	buf := make([]byte, align)
	n, err := r.Read(buf)
	if err != nil {
		return xerrors.Errorf("failed to read buf: %w", err)
	}
	if n != align {
		return xerrors.Errorf("read length error: %d", n)
	}

	if err := binary.Read(bytes.NewReader(buf), o, v); err != nil {
		return xerrors.Errorf("failed to read binary: %w", err)
	}

	return nil
}
