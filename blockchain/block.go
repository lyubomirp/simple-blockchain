package blockchain

import (
	"bytes"
	"crypto/sha256"
	"encoding/gob"
	"log"
)

// Block is a single block in the chain. Its Hash commits to PrevHash, to the
// transactions the block carries and to the Nonce found by proof of work.
type Block struct {
	Hash         []byte
	Transactions []*Transaction
	PrevHash     []byte
	Nonce        int
}

// CreateBlock assembles a block holding txs on top of prevHash and runs proof
// of work to find its nonce and hash. It blocks until the work is done.
func CreateBlock(txs []*Transaction, prevHash []byte) *Block {
	block := &Block{[]byte{}, txs, prevHash, 0}

	pow := NewProof(block)
	nonce, hash := pow.Run()

	block.Hash = hash[:]
	block.Nonce = nonce

	return block
}

// Genesis creates the first block of a chain. It carries only coinbase and has
// no previous hash.
func Genesis(coinbase *Transaction) *Block {
	return CreateBlock([]*Transaction{coinbase}, []byte{})
}

// Serialize encodes b as gob for storage in the database. It panics if the
// block cannot be encoded.
func (b *Block) Serialize() []byte {
	var res bytes.Buffer
	encoder := gob.NewEncoder(&res)

	err := encoder.Encode(b)

	Handle(err)

	return res.Bytes()

}

// Deserialize decodes a block previously encoded by Block.Serialize. It panics
// if data is not a valid gob-encoded block.
func Deserialize(data []byte) *Block {
	var block Block
	decoder := gob.NewDecoder(bytes.NewReader(data))

	err := decoder.Decode(&block)

	Handle(err)

	return &block
}

// Handle panics if err is non-nil. The package uses it in place of returning
// errors to the caller.
func Handle(err error) {
	if err != nil {
		log.Panic(err)
	}
}

// HashTransactions returns a SHA-256 hash over the IDs of the block's
// transactions, committing the block to its contents.
func (b *Block) HashTransactions() []byte {
	var txHashes [][]byte
	var txHash [32]byte

	for _, txH := range b.Transactions {
		txHashes = append(txHashes, txH.ID)
	}

	txHash = sha256.Sum256(bytes.Join(txHashes, []byte{}))

	return txHash[:]
}
