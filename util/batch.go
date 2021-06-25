package util

type batchProcFn func(idx int, thread int) error

// BatchIndices processes a range of indices in parallel.  Each index is passed
// to the specified callback.

// start: The first index to process.
// count: The number of indices to process.
// numThreads: The number of go routines to use.
// proc: The callback to pass each index to.
func BatchIndices(start int, count int, numThreads int, proc batchProcFn) error {
	idxch := make(chan int)
	donech := make(chan error)
	abortch := make(chan struct{})

	for i := 0; i < numThreads; i++ {
		go func(thread int) {
			for idx := range idxch {
				select {
				case <-abortch:
					donech <- nil
					return

				default:
					err := proc(idx, thread)
					if err != nil {
						donech <- err
						return
					}
				}
			}

			donech <- nil
		}(i)
	}

	for i := 0; i < count; i++ {
		idxch <- start + i
	}
	close(idxch)

	var firstErr error
	for i := 0; i < numThreads; i++ {
		err := <-donech
		if err != nil && firstErr == nil {
			close(abortch)
			firstErr = err
		}
	}

	return firstErr
}
