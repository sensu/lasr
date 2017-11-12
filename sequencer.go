package lasr

import "github.com/boltdb/bolt"

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

func (q *Q) nextSequence(tx *bolt.Tx) (ID, error) {
	if q.seq != nil {
		return q.seq.NextSequence()
	}
	return q.nextUint64ID(tx)
}

func (q *Q) nextUint64ID(tx *bolt.Tx) (Uint64ID, error) {
	bucket := tx.Bucket(q.name)
	seq, err := bucket.NextSequence()

	if err != nil {
		return Uint64ID(0), err
	}

	return Uint64ID(seq), nil
}