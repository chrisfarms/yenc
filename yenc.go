// Package yenc
// decoder for yenc encoded binaries (yenc.org)
package yenc

import (
	"bufio"
	"bytes"
	"fmt"
	"hash"
	"hash/crc32"
	"io"
	"strconv"
	"strings"
)

func parseHeaders(inputBytes []byte) map[string]string {
	values := make(map[string]string)
	input := string(inputBytes)
	// get the filename name off the end
	ni := strings.Index(input, "name=")
	if ni > -1 {
		values["name"] = input[ni:]
	} else {
		ni = len(input)
	}
	// get other header values
	for _, header := range strings.Split(input[:ni], " ") {
		kv := strings.SplitN(strings.TrimSpace(header), "=", 2)
		if len(kv) < 2 {
			continue
		}
		values[kv[0]] = kv[1]
	}
	// done
	return values
}

type Part struct {
	// part num
	Number int
	// size from header
	hsize int64
	// size from part trailer
	Size int64
	// file boundarys
	Begin, End int64
	// filename from yenc header
	Name string
	// line length of part
	cols int
	// crc check for this part
	crc32   uint32
	crcHash hash.Hash32
	// the decoded data
	Body []byte
}

func (p *Part) validate() error {
	// length checks
	if int64(len(p.Body)) != p.Size {
		return fmt.Errorf("Body size %d did not match expected size %d", len(p.Body), p.Size)
	}
	// crc check
	if p.crc32 > 0 {
		if sum := p.crcHash.Sum32(); sum != p.crc32 {
			return fmt.Errorf("crc check failed for part %d expected %x got %x", p.Number, p.crc32, sum)
		}
	}
	return nil
}

type decoder struct {
	// the buffered input
	buf *bufio.Reader
	// whether we are decoding multipart
	multipart bool
	// numer of parts if given
	total int
	// list of parts
	parts []*Part
	// active part
	part *Part
	// overall crc check
	crc32   uint32
	crcHash hash.Hash32
	// are we waiting for an escaped char
	awaitingSpecial bool
}

func (d *decoder) validate() error {
	if d.crc32 > 0 {
		if sum := d.crcHash.Sum32(); sum != d.crc32 {
			return fmt.Errorf("crc check failed expected %x got %x", d.crc32, sum)
		}
	}
	return nil
}

func (d *decoder) readHeader() (err error) {
	var s string
	// find the start of the header
	for {
		s, err = d.buf.ReadString('\n')
		if err != nil {
			return io.EOF
		}
		if len(s) >= 7 && s[:7] == "=ybegin" {
			break
		}
	}
	// split on name= to get name first
	parts := strings.SplitN(s[7:], "name=", 2)
	if len(parts) > 1 {
		d.part.Name = strings.TrimSpace(parts[1])
	}
	// split on sapce for other headers
	parts = strings.Split(parts[0], " ")
	for i, _ := range parts {
		kv := strings.Split(strings.TrimSpace(parts[i]), "=")
		if len(kv) < 2 {
			continue
		}
		switch kv[0] {
		case "size":
			d.part.hsize, _ = strconv.ParseInt(kv[1], 10, 64)
		case "line":
			d.part.cols, _ = strconv.Atoi(kv[1])
		case "part":
			d.part.Number, _ = strconv.Atoi(kv[1])
			d.multipart = true
		case "total":
			d.total, _ = strconv.Atoi(kv[1])
		}
	}
	return nil
}

func (d *decoder) readPartHeader() (err error) {
	var s string
	// find the start of the header
	for {
		s, err = d.buf.ReadString('\n')
		if err != nil {
			return err
		}
		if len(s) >= 6 && s[:6] == "=ypart" {
			break
		}
	}
	// split on space for headers
	parts := strings.Split(s[6:], " ")
	for i, _ := range parts {
		kv := strings.Split(strings.TrimSpace(parts[i]), "=")
		if len(kv) < 2 {
			continue
		}
		switch kv[0] {
		case "begin":
			d.part.Begin, _ = strconv.ParseInt(kv[1], 10, 64)
		case "end":
			d.part.End, _ = strconv.ParseInt(kv[1], 10, 64)
		}
	}
	return nil
}

func (d *decoder) parseTrailer(line string) error {
	// split on space for headers
	parts := strings.Split(line, " ")
	for i, _ := range parts {
		kv := strings.Split(strings.TrimSpace(parts[i]), "=")
		if len(kv) < 2 {
			continue
		}
		switch kv[0] {
		case "size":
			d.part.Size, _ = strconv.ParseInt(kv[1], 10, 64)
		case "pcrc32":
			if crc64, err := strconv.ParseUint(kv[1], 16, 64); err == nil {
				d.part.crc32 = uint32(crc64)
			}
		case "crc32":
			if crc64, err := strconv.ParseUint(kv[1], 16, 64); err == nil {
				d.crc32 = uint32(crc64)
			}
		case "part":
			partNum, _ := strconv.Atoi(kv[1])
			if partNum != d.part.Number {
				return fmt.Errorf("yenc: =yend header out of order expected part %d got %d", d.part.Number, partNum)
			}
		}
	}
	return nil
}

func (d *decoder) decode(line []byte) []byte {
	i, j := 0, 0
	for ; i < len(line); i, j = i+1, j+1 {
		// escaped chars yenc42+yenc64
		if d.awaitingSpecial {
			line[j] = (((line[i] - 42) & 255) - 64) & 255
			d.awaitingSpecial = false
			// if escape char - then skip and backtrack j
		} else if line[i] == '=' {
			d.awaitingSpecial = true
			j--
			continue
			// normal char, yenc42
		} else {
			line[j] = (line[i] - 42) & 255
		}
	}
	// return the new (possibly shorter) slice
	// shorter because of the escaped chars
	return line[:len(line)-(i-j)]
}

func (d *decoder) readBody() error {
	// ready the part body 
	d.part.Body = make([]byte, 0)
	// reset special
	d.awaitingSpecial = false
	// setup crc hash
	d.part.crcHash = crc32.NewIEEE()
	// each line
	for {
		line, err := d.buf.ReadBytes('\n')
		if err != nil {
			return err
		}
		// strip linefeeds (some use CRLF some LF)
		line = bytes.TrimRight(line, "\r\n")
		// check for =yend
		if len(line) >= 5 && string(line[:5]) == "=yend" {
			return d.parseTrailer(string(line))
		}
		// decode
		b := d.decode(line)
		// update hashs
		d.part.crcHash.Write(b)
		d.crcHash.Write(b)
		// decode
		d.part.Body = append(d.part.Body, b...)
	}
	return nil
}

func (d *decoder) run() error {
	// init hash
	d.crcHash = crc32.NewIEEE()
	// for each part
	for {
		// create a part
		d.part = new(Part)
		// read the header
		if err := d.readHeader(); err != nil {
			return err
		}
		// read part header if available
		if d.multipart {
			if err := d.readPartHeader(); err != nil {
				return err
			}
		}
		// decode the part body
		if err := d.readBody(); err != nil {
			return err
		}
		// add part to list
		d.parts = append(d.parts, d.part)
		// validate part
		if err := d.part.validate(); err != nil {
			return err
		}
	}
	return nil
}

// return a single part from yenc data
func Decode(input io.Reader) (*Part, error) {
	d := &decoder{buf: bufio.NewReader(input)}
	if err := d.run(); err != nil && err != io.EOF {
		return nil, err
	}
	if len(d.parts) == 0 {
		return nil, fmt.Errorf("no yenc parts found")
	}
	// validate multipart only if all parts are present
	if !d.multipart || len(d.parts) == d.parts[len(d.parts)-1].Number {
		if err := d.validate(); err != nil {
			return nil, err
		}
	}
	return d.parts[0], nil
}
