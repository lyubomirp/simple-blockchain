package blockchain

import (
	"bytes"
	"crypto/sha256"
	"encoding/binary"
	"fmt"
	"math"
	"math/big"
)

// diff is the mining difficulty: the number of leading zero bits a block hash
// must have to be accepted.
const diff = 16

// ProofOfWork pairs a block with the target value its hash must fall below for
// the block to count as mined.
type ProofOfWork struct {
	Block  *Block
	Target *big.Int
}

// NewProof returns the proof of work for b, with a target derived from the
// package difficulty.
func NewProof(b *Block) *ProofOfWork {
	target := big.NewInt(1)
	target.Lsh(target, uint(256-diff))

	pow := &ProofOfWork{Block: b, Target: target}

	return pow
}

// InitData returns the bytes hashed for the given nonce: the previous hash, the
// block's transaction hash, the nonce and the difficulty.
func (pow *ProofOfWork) InitData(nonce int) []byte {
	data := bytes.Join(
		[][]byte{
			pow.Block.PrevHash,
			pow.Block.HashTransactions(),
			ToHex(int64(nonce)),
			ToHex(int64(diff)),
		},
		[]byte{},
	)

	return data
}

// Run searches for a nonce whose hash falls below the target and returns that
// nonce with the hash it produced. It prints every candidate hash to standard
// output as it searches.
func (pow *ProofOfWork) Run() (int, []byte) {
	var intHash big.Int
	var hash [32]byte
	nonce := 0

	for nonce < math.MaxInt64 {
		data := pow.InitData(nonce)
		hash = sha256.Sum256(data)

		fmt.Printf("\r%x", hash)
		intHash.SetBytes(hash[:])

		if intHash.Cmp(pow.Target) == -1 {
			break
		} else {
			nonce++
		}
	}

	fmt.Println()

	return nonce, hash[:]
}

// Validate reports whether the nonce recorded on the block really does hash
// below the target.
func (pow *ProofOfWork) Validate() bool {
	var intHash big.Int

	data := pow.InitData(pow.Block.Nonce)
	hash := sha256.Sum256(data)
	intHash.SetBytes(hash[:])

	return intHash.Cmp(pow.Target) == -1
}

// ToHex returns the big-endian 8-byte representation of num.
func ToHex(num int64) []byte {
	var buf [8]byte
	binary.BigEndian.PutUint64(buf[:], uint64(num))

	return buf[:]
}
