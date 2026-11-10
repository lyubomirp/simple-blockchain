package blockchain

import (
	"bytes"
	"crypto/sha256"
	"encoding/gob"
	"fmt"
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

// Serialize encodes b as gob for storage in the database.
func (b *Block) Serialize() ([]byte, error) {
	var res bytes.Buffer

	if err := gob.NewEncoder(&res).Encode(b); err != nil {
		return nil, fmt.Errorf("encode block: %w", err)
	}

	return res.Bytes(), nil
}

// Deserialize decodes a block previously encoded by Block.Serialize.
func Deserialize(data []byte) (*Block, error) {
	var block Block

	if err := gob.NewDecoder(bytes.NewReader(data)).Decode(&block); err != nil {
		return nil, fmt.Errorf("decode block: %w", err)
	}

	return &block, nil
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
