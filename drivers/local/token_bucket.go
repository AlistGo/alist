package local

import "context"

type TokenBucket interface {
	Take() <-chan struct{}
	Put()
	Do(context.Context, func() error) error
}

// StaticTokenBucket is a bucket with a fixed number of tokens,
// where the retrieval and return of tokens are manually controlled.
// In the initial state, the bucket is full.
type StaticTokenBucket struct {
	bucket chan struct{}
}

func NewStaticTokenBucket(size int) StaticTokenBucket {
	bucket := make(chan struct{}, size)
	for range size {
		bucket <- struct{}{}
	}
	return StaticTokenBucket{bucket: bucket}
}

func NewStaticTokenBucketWithMigration(oldBucket TokenBucket, size int) StaticTokenBucket {
	if oldBucket != nil {
		oldStaticBucket, ok := oldBucket.(StaticTokenBucket)
		if ok {
			oldSize := cap(oldStaticBucket.bucket)
			migrateSize := oldSize
			if size < migrateSize {
				migrateSize = size
			}

			bucket := make(chan struct{}, size)
			for range size - migrateSize {
				bucket <- struct{}{}
			}

			if migrateSize != 0 {
				go func() {
					for range migrateSize {
						<-oldStaticBucket.bucket
						bucket <- struct{}{}
					}
					close(oldStaticBucket.bucket)
				}()
			}
			return StaticTokenBucket{bucket: bucket}
		}
	}
	return NewStaticTokenBucket(size)
}

// Take channel maybe closed when local driver is modified.
// don't call Put method after the channel is closed.
func (b StaticTokenBucket) Take() <-chan struct{} {
	return b.bucket
}

func (b StaticTokenBucket) Put() {
	b.bucket <- struct{}{}
}

func (b StaticTokenBucket) Do(ctx context.Context, f func() error) error {
	select {
	case <-ctx.Done():
		return ctx.Err()
	case _, ok := <-b.Take():
		if ok {
			defer b.Put()
		}
	}
	return f()
}

// NopTokenBucket all function calls to this bucket will success immediately
type NopTokenBucket struct {
	nop chan struct{}
}

func NewNopTokenBucket() NopTokenBucket {
	nop := make(chan struct{})
	close(nop)
	return NopTokenBucket{nop}
}

func (b NopTokenBucket) Take() <-chan struct{} {
	return b.nop
}

func (b NopTokenBucket) Put() {}

func (b NopTokenBucket) Do(_ context.Context, f func() error) error { return f() }
