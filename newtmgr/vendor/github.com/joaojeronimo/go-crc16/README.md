# crc16 for Go
## Why

The [hash package](http://golang.org/pkg/hash/) only provides crc32 and crc64 functions, and I needed crc16 for consistent hashing in the Redis Cluster Client that I'm going to build next (just a wrapper of [radix](https://github.com/fzzbt/radix/)).

## Usage

```go
package main

import (
	"github.com/joaojeronimo/go-crc16"
	"fmt"
)

func main () {
	fmt.Println( crc16.Crc16("Hello World") )
	fmt.Println( crc16.Kermit("Hello World") )
}
```

## License

(The MIT License)

Copyright (c) 2012 João Jerónimo jj@crowdprocess.com

Permission is hereby granted, free of charge, to any person obtaining a copy of this software and associated documentation files (the 'Software'), to deal in the Software without restriction, including without limitation the rights to use, copy, modify, merge, publish, distribute, sublicense, and/or sell copies of the Software, and to permit persons to whom the Software is furnished to do so, subject to the following conditions:

The above copyright notice and this permission notice shall be included in all copies or substantial portions of the Software.

THE SOFTWARE IS PROVIDED 'AS IS', WITHOUT WARRANTY OF ANY KIND, EXPRESS OR IMPLIED, INCLUDING BUT NOT LIMITED TO THE WARRANTIES OF MERCHANTABILITY, FITNESS FOR A PARTICULAR PURPOSE AND NONINFRINGEMENT. IN NO EVENT SHALL THE AUTHORS OR COPYRIGHT HOLDERS BE LIABLE FOR ANY CLAIM, DAMAGES OR OTHER LIABILITY, WHETHER IN AN ACTION OF CONTRACT, TORT OR OTHERWISE, ARISING FROM, OUT OF OR IN CONNECTION WITH THE SOFTWARE OR THE USE OR OTHER DEALINGS IN THE SOFTWARE.