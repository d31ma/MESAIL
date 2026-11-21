package mailout

import "sync/atomic"

type WriteCounter struct {
	Documents Documents
	written   *atomic.Int64
}

func NewWriteCounter(documents Documents) WriteCounter {
	return WriteCounter{Documents: documents, written: &atomic.Int64{}}
}

func (counter WriteCounter) Written() int {
	return int(counter.written.Load())
}

func (counter WriteCounter) CollectionExists(name string) (bool, error) {
	return counter.Documents.CollectionExists(name)
}

func (counter WriteCounter) CreateCollection(name string) error {
	counter.written.Add(1)
	return counter.Documents.CreateCollection(name)
}

func (counter WriteCounter) Insert(collection string, document any) (string, error) {
	counter.written.Add(1)
	return counter.Documents.Insert(collection, document)
}

func (counter WriteCounter) FindById(collection string, id string, target any) (bool, error) {
	return counter.Documents.FindById(collection, id, target)
}

func (counter WriteCounter) Patch(collection string, id string, fields any, target any) (bool, error) {
	counter.written.Add(1)
	return counter.Documents.Patch(collection, id, fields, target)
}

func (counter WriteCounter) Command(command map[string]any) (any, error) {
	return counter.Documents.Command(command)
}
