package wal

import (
	"encoding/binary"
	"errors"
	"hash/crc32"
	"io"
)

// Record format: [length: uint32][crc32: uint32][payload: []byte]
const headerSize = 8 // 4 bytes length + 4 bytes CRC32

const maxRecordPayload = 64 * 1024 * 1024 // 64 MiB

var (
	ErrCorruptRecord  = errors.New("wal: corrupt record (CRC mismatch)")
	ErrTruncated      = errors.New("wal: truncated record")
	ErrRecordTooLarge = errors.New("wal: record exceeds maximum size")
)

// encodeRecord writes a length-prefixed, CRC32-checksummed record to w.
func encodeRecord(w io.Writer, payload []byte) (int, error) {
	var hdr [headerSize]byte
	binary.LittleEndian.PutUint32(hdr[0:4], uint32(len(payload)))
	binary.LittleEndian.PutUint32(hdr[4:8], crc32.ChecksumIEEE(payload))

	n, err := w.Write(hdr[:])
	if err != nil {
		return n, err
	}
	n2, err := w.Write(payload)
	return n + n2, err
}

// decodeRecord reads a single record from r. Returns the payload or an error.
// Returns io.EOF when no more data is available.
// Returns ErrTruncated if the record header or payload is incomplete.
// Returns ErrCorruptRecord if the CRC does not match.
func decodeRecord(r io.Reader) ([]byte, error) {
	var hdr [headerSize]byte
	if _, err := io.ReadFull(r, hdr[:]); err != nil {
		if err == io.ErrUnexpectedEOF {
			return nil, ErrTruncated
		}
		return nil, err // io.EOF on clean boundary
	}

	length := binary.LittleEndian.Uint32(hdr[0:4])
	checksum := binary.LittleEndian.Uint32(hdr[4:8])

	if length > maxRecordPayload {
		return nil, ErrRecordTooLarge
	}

	payload := make([]byte, length)
	if _, err := io.ReadFull(r, payload); err != nil {
		if err == io.ErrUnexpectedEOF || err == io.EOF {
			return nil, ErrTruncated
		}
		return nil, err
	}

	if crc32.ChecksumIEEE(payload) != checksum {
		return nil, ErrCorruptRecord
	}
	return payload, nil
}

// recordSize returns the total on-disk size of a record with the given payload.
func recordSize(payloadLen int) int64 {
	return int64(headerSize + payloadLen)
}
