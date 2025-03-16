package events

import (
	"encoding/binary"
	"errors"
	"fmt"
)

type LRCEvent = []byte
type LRCData = []byte
type LRCTypedData = []byte
type LRCServerEvent = []byte

// EventType determines how a command on the LRC protocol should be interpreted
type EventType uint8

var ServerPong = []byte{6, 0, 0, 0, 0, 1}
var ServerPing = []byte{6, 0, 0, 0, 0, 1}
var ClientPong = []byte{2, 1}
var ClientPing = []byte{2, 0}

const (
	EventPing       EventType = iota // EventPing is a request for a pong, and if it comes from a server, it can also contain a welcome message
	EventPong                        // EventPong determines the latency of the connection, and if the connection has closed
	EventInit                        // EventInit initializes a message
	EventPub                         // EventPub publishes a message
	EventInsert                      // EventInsert inserts a character at a specified position in a message
	EventDelete                      // EventDelete deletes a character at a specified position in a message
	EventMuteUser                    // EventMuteUser mutes a user based on a message id. only works going forward
	EventUnmuteUser                  // EventUnmuteUser unmutes a user based on a post id. only works going forward
)

// IsPing returns true if e is a ping event
func IsPing(td LRCTypedData) bool {
	if len([]byte(td)) == 0 {
		return false
	}
	return td[0] == byte(EventPing)
}

// IsPub returns true if e is a publish event
func IsPub(td LRCTypedData) bool {
	if len([]byte(td)) == 0 {
		return false
	}
	return td[0] == byte(EventPub)
}

// IsInit returns true if e is an initialize event
func IsInit(td LRCTypedData) bool {
	if len([]byte(td)) == 0 {
		return false
	}
	return td[0] == byte(EventInit)
}

// GenServerEvent returns an LRCServerEvent from data and an id
func GenServerEvent(data LRCTypedData, id uint32) (LRCServerEvent, LRCServerEvent) {
	se := make([]byte, 4)
	binary.BigEndian.PutUint32(se, id)
	se = append(se, data...)
	PrependLength(&se)
	ee := se
	if IsInit(data) {
		ee = make([]byte, len(se))
		copy(ee, se)
		ee[6] = 1
	}
	return se, ee
}

func GenInitEvent(color uint8, name string) LRCEvent {
	e := []byte{byte(EventInit), 0, color}
	e = append(e, []byte(name)...)
	PrependLength(&e)
	return e
}

func GenPubEvent() LRCEvent {
	e := []byte{byte(EventPub)}
	PrependLength(&e)
	return e
}

func GenInsertEvent(at uint16, s string) LRCEvent {
	e := []byte{byte(EventInsert)}
	a := make([]byte, 2)
	binary.BigEndian.PutUint16(a, at)
	e = append(e, a...)
	e = append(e, []byte(s)...)
	PrependLength(&e)
	return e
}

func GenDeleteEvent(at uint16) LRCEvent {
	e := []byte{byte(EventDelete)}
	a := make([]byte, 2)
	binary.BigEndian.PutUint16(a, at)
	e = append(e, a...)
	PrependLength(&e)
	return e
}

// PrependLength prepends the length of the data
func PrependLength(data *[]byte) {
	l := len(*data) + 1
	n := make([]byte, 1, l)
	n[0] = byte(l)
	*data = append(n, *data...)
}

func ParseEventType(e LRCEvent) EventType {
	return EventType(e[4])
}

func ParseInitEvent(e LRCEvent) (uint32, uint8, string, bool) {
	return binary.BigEndian.Uint32(e[0:4]), e[6], string(e[7:]), e[5]==1
}

func ParsePubEvent(e LRCEvent) uint32 {
	return binary.BigEndian.Uint32(e[0:4])
}

func ParseInsertEvent(e LRCEvent) (uint32, uint16, string) {
	return binary.BigEndian.Uint32(e[0:4]), binary.BigEndian.Uint16(e[5:7]), string(e[7])
}

func ParseDeleteEvent(e LRCEvent) (uint32, uint16) {
	return binary.BigEndian.Uint32(e[0:4]), binary.BigEndian.Uint16(e[5:7])
}

type ringBuffer struct {
	buffer [][]byte
	head   int
	tail   int
	data   int
	cap    int
}

func newRingBuffer(capacity int) *ringBuffer {
	if capacity < 1 {
		panic("capacity must be at least 1")
	}
	return &ringBuffer{
		buffer: make([][]byte, capacity),
		cap:    capacity,
	}
}

func (rb *ringBuffer) length() int {
	return (rb.tail - rb.head + rb.cap) % rb.cap
}

func (rb *ringBuffer) enqueue(e []byte) error {
	if rb.length() == rb.cap-1 {
		return errors.New("buffer full")
	}
	rb.buffer[rb.tail] = e
	rb.data += len(e)
	rb.tail = (rb.tail + 1) % rb.cap
	return nil
}

func (rb *ringBuffer) dequeue(target byte) ([]byte, error) {
	if rb.data < int(target) {
		return nil, fmt.Errorf("cannot dequeue %d bytes from %d bytes", target, rb.data)
	}
	result := make([]byte, 0, target)

	for len(result) < int(target) {
		front := rb.buffer[rb.head]
		needed := int(target) - len(result)

		if len(front) <= needed {
			result = append(result, front...)
			rb.buffer[rb.head] = nil
			rb.head = (rb.head + 1) % rb.cap
		} else {
			result = append(result, front[:needed]...)
			rb.buffer[rb.head] = front[needed:]
		}
	}
	rb.data -= int(target)
	result = result[1:]
	return result, nil
}

func (rb *ringBuffer) getTarget() (byte, error) {
	if rb.length() == 0 {
		panic("should not get target on empty buffer")
	}
	target := rb.buffer[rb.head][0]
	if target == 0 {
		return 0, errors.New("event length 0")
	}
	return target, nil
}

// Degunker recieves a channel in of bytes that we read from the tcp connection, which may correspond to less than one, just one, or more than one lrc events.
// It stores up to capacity of these events, and whenever it stores enough data corresponding to an LRCEvent, it sends it out on the out channel.
// If something goes wrong, (it runs out of capacity, it recieves an event length 0,) it closes the quit channel, and returns an error.
// It will return with no error if the in channel closes. It will panic if the out channel closes.
func Degunker(capacity int, in chan []byte, out chan LRCEvent, quit chan struct{}) error {
	var target byte
	var err error
	rb := newRingBuffer(capacity)
	for {
		b, ok := <-in
		if !ok {
			close(quit)
			return nil
		}

		err = rb.enqueue(b)
		if err != nil {
			close(quit)
			return err
		}

		for int(target) <= rb.data {
			if target == 0 {
				if rb.length() == 0 {
					break
				}
				target, err = rb.getTarget()
				if err != nil {
					close(quit)
					return err
				}
				if int(target) > rb.data {
					break
				}
			}
			evt, err := rb.dequeue(target)
			if err != nil {
				close(quit)
				return err
			}

			out <- evt
			target = 0
		}
	}
}
