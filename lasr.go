package lasr

import (
	"bytes"
	"encoding"
	"encoding/binary"
	"errors"
	"fmt"
	"sync"
	"sync/atomic"
)

var (
	emptyQ     = errors.New("empty queue")
	ErrAckNack = errors.New("lasr: Ack or Nack already called")
	ErrQClosed = errors.New("lasr: Q is closed")
)

// ID is used for bolt keys. Every message will be assigned an ID.
type ID interface {
	encoding.BinaryMarshaler
}

// Uint64ID is the default ID used by lasr.
type Uint64ID uint64

func (id Uint64ID) MarshalBinary() ([]byte, error) {
	buf := bytes.NewBuffer(make([]byte, 8))
	err := binary.Write(buf, binary.BigEndian, id)
	return buf.Bytes(), err
}

type Status string

const (
	Ready    Status = "Ready"
	Unacked  Status = "Unacked"
	Returned Status = "Returned"
)

type Message struct {
	Body []byte
	ID   []byte
	q    *Q
	once int32
	err  error
}

// Sequencer returns an ID with each call to NextSequence and any error
// that occurred.
//
// A Sequencer should obey the following invariants:
//
// * NextSequence is goroutine-safe.
//
// * NextSequence will never generate the same ID.
//
// * NextSequence will return IDs whose big-endian binary representation is incrementing.
//
// Q is not guaranteed to use all of the IDs generated by its Sequencer.
type Sequencer interface {
	NextSequence() (ID, error)
}

// Options can be passed to NewQ.
type Option func(q *Q) error

// WithSequencer will cause a Q to use a user-provided Sequencer.
func WithSequencer(seq Sequencer) Option {
	return func(q *Q) error {
		q.seq = seq
		return nil
	}
}

// WithDeadLetters will cause nacked messages that are not retried to be added
// to a dead letters queue.
func WithDeadLetters() Option {
	return func(q *Q) error {
		q.returnedKey = []byte("deadletters")
		return nil
	}
}

// WithMessageBufferSize sets the message buffer size. By default, the message
// buffer size is 0. Values less than 0 are not allowed.
//
// The buffer is used by Receive to efficiently ready messages for consumption.
// If the buffer is greater than 0, then multiple messages can retrieved in a
// single transaction.
//
// Buffered messages come with a caveat: messages will move into the "unacked"
// state before Receive is called.
//
// Buffered messages come at the cost of increased memory use. If messages are
// large in size, use this cautiously.
func WithMessageBufferSize(size int) Option {
	return func(q *Q) error {
		if size < 0 {
			return fmt.Errorf("lasr: invalid message buffer size: %d", size)
		}
		q.messagesBufSize = size
		return nil
	}
}

// Ack acknowledges successful receipt and processing of the Message.
func (m *Message) Ack() (err error) {
	if !atomic.CompareAndSwapInt32(&m.once, 0, 1) {
		return ErrAckNack
	}
	return m.q.ack(m.ID)
}

// Nack negatively acknowledges successful receipt and processing of the
// Message. If Nack is called with retry True, then the Message will be
// placed back in the queue in its original position.
func (m *Message) Nack(retry bool) (err error) {
	if !atomic.CompareAndSwapInt32(&m.once, 0, 1) {
		return ErrAckNack
	}
	return m.q.nack(m.ID, retry)
}

// fifo is for buffering received messages
type fifo struct {
	data []*Message
	sync.Mutex
}

func newFifo(size int) *fifo {
	return &fifo{
		data: make([]*Message, 0, size),
	}
}

func (f *fifo) Pop() *Message {
	msg := f.data[0]
	f.data = append(f.data[0:0], f.data[1:]...)
	return msg
}

func (f *fifo) Push(m *Message) {
	if len(f.data) == cap(f.data) {
		panic("push to full buffer")
	}
	f.data = append(f.data, m)
}

func (f *fifo) Len() int {
	return len(f.data)
}
