yenc.go
=======

A single/multipart yenc decoder. Usually used for binary files stored on Usenet servers.

Installation
------------

The easy way:

`go get github.com/chrisfarms/yenc`

Docs
----

Decode accepts an io.Reader (of yenc encoded data - complete with headers)
and returns a *Part.

```go
func Decode(input io.Reader) (*Part, error)
```

The Part struct contains all the decoded data.

```go
type Part struct {

    // part num
    Number int

    // size from part trailer
    Size int
    
    // file boundarys
    Begin, End int
    
    // filename from yenc header
    Name string

    // the decoded data
    Body []byte
    
    // ..contains filtered or unexported fields..
}
```

Example
-------

```go
package main
import (
	"github.com/chrisfarms/yenc"
	"os"
)
func main(){
	f,err := os.Open("some_file.yenc")
	if err != nil {
	    panic("could not open file")
	}
	part,err := yenc.Decode(f)
	if err != nil {
	    panic("decoding: " + err.Error())
	}
	fmt.Println("Filename", part.Name)
	fmt.Println("Body Bytes", part.Body)
}
```
