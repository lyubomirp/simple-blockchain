// Package blockchain implements a minimal proof-of-work blockchain with a
// UTXO-based token system, persisted in a local Badger key-value store.
package blockchain

import (
	"encoding/hex"
	"fmt"
	"os"
	"runtime"

	"github.com/dgraph-io/badger"
)

const (
	// path is the directory holding the Badger database files.
	path = "./tmp/blocks"
	// dbFile is a file Badger always creates, used to detect an existing chain.
	dbFile = "./tmp/blocks/MANIFEST"
	// genesis is the note recorded in the genesis block's coinbase transaction.
	genesis = "First transaction from Genesis"
)

// BlockChain is a chain of blocks backed by a Badger database. The zero value
// is not usable: obtain a BlockChain from InitBlockChain or ContinueBlockChain.
// The caller owns Database and is responsible for closing it.
type BlockChain struct {
	// LastHash is the hash of the most recently added block.
	LastHash []byte
	// Database stores serialized blocks keyed by block hash, plus the key "lh"
	// holding the last hash.
	Database *badger.DB
}

// BlockChainIterator walks a chain backwards, from its newest block towards
// genesis. Use BlockChain.Iterator to create one.
type BlockChainIterator struct {
	CurrentHash []byte
	Database    *badger.DB
}

// InitBlockChain creates a new blockchain in the local database directory and
// mines its genesis block, paying the coinbase reward to address. If a chain
// already exists it prints a message and ends the calling goroutine with
// runtime.Goexit. It panics on any database error.
func InitBlockChain(address string) *BlockChain {
	var lastHash []byte

	_, err := os.Stat(path)

	if DBExists() {
		fmt.Println("Blackchain already exists")
		runtime.Goexit()
	}

	if os.IsNotExist(err) {
		fmt.Printf("Creating %v directory \r\n", path)
		err := os.MkdirAll(path, os.ModePerm)

		if err != nil {
			panic(fmt.Sprintf("Cannot create %s directory", path))
		}
	}

	opts := badger.DefaultOptions(path)
	opts.Logger = nil

	db, err := badger.Open(opts)

	Handle(err)

	err = db.Update(func(txn *badger.Txn) error {
		cbtx := CoinbaseTx(address, genesis)
		genesisBlock := Genesis(cbtx)

		fmt.Println("Genesis made")

		err := txn.Set(genesisBlock.Hash, genesisBlock.Serialize())

		Handle(err)

		err = txn.Set([]byte("lh"), genesisBlock.Hash)

		lastHash = genesisBlock.Hash

		return err

	})

	Handle(err)

	blockchain := BlockChain{lastHash, db}
	return &blockchain
}

// DBExists reports whether a blockchain database already exists on disk.
func DBExists() bool {
	if _, err := os.Stat(dbFile); os.IsNotExist(err) {
		return false
	}
	return true
}

// ContinueBlockChain opens the existing blockchain database and returns a chain
// positioned at its last block. The address argument is unused and kept only
// for symmetry with InitBlockChain. If no chain exists it prints a message and
// ends the calling goroutine with runtime.Goexit.
func ContinueBlockChain(address string) *BlockChain {
	if !DBExists() {
		fmt.Println("No existing blockchain found. Create one!")
		runtime.Goexit()
	}

	var lastHash []byte

	opts := badger.DefaultOptions(path)
	opts.Logger = nil

	db, err := badger.Open(opts)
	Handle(err)

	err = db.Update(func(tx *badger.Txn) error {
		item, err := tx.Get([]byte("lh"))
		Handle(err)

		err = item.Value(func(val []byte) error {
			lastHash = append(lastHash, val...)
			return nil
		})

		return err
	})

	Handle(err)

	chain := BlockChain{lastHash, db}

	return &chain
}

// AddBlock mines a block containing transactions, appends it to the chain and
// records it as the new last hash. It panics on any database error.
func (chain *BlockChain) AddBlock(transactions []*Transaction) {
	var lastHash []byte

	err := chain.Database.View(func(txn *badger.Txn) error {
		item, err := txn.Get([]byte("lh"))
		Handle(err)

		err = item.Value(func(val []byte) error {
			lastHash = append(lastHash, val...)
			return nil
		})

		return err
	})

	Handle(err)
	newBlock := CreateBlock(transactions, lastHash)

	err = chain.Database.Update(func(txn *badger.Txn) error {
		err := txn.Set(newBlock.Hash, newBlock.Serialize())
		Handle(err)
		err = txn.Set([]byte("lh"), newBlock.Hash)
		chain.LastHash = newBlock.Hash
		return err
	})

	Handle(err)
}

// Iterator returns an iterator positioned at the chain's last block.
func (chain *BlockChain) Iterator() *BlockChainIterator {
	iter := &BlockChainIterator{chain.LastHash, chain.Database}

	return iter
}

// Next returns the block at the iterator's current position and moves the
// iterator one block closer to genesis. Callers stop once the returned block
// has an empty PrevHash; calling Next after that panics.
func (iter *BlockChainIterator) Next() *Block {
	var block *Block

	err := iter.Database.View(func(txn *badger.Txn) error {
		var encodedBlock []byte

		item, err := txn.Get(iter.CurrentHash)
		Handle(err)

		err = item.Value(func(val []byte) error {
			encodedBlock = append(encodedBlock, val...)
			return nil
		})
		block = Deserialize(encodedBlock)

		return err
	})

	Handle(err)

	iter.CurrentHash = block.PrevHash

	return block
}

// FindUnspentTransactions returns the transactions that still hold at least one
// output unlockable by address.
func (chain *BlockChain) FindUnspentTransactions(address string) []Transaction {
	var unspent []Transaction

	spent := make(map[string][]int)

	iter := chain.Iterator()

	for {
		block := iter.Next()

		for _, tx := range block.Transactions {
			txId := hex.EncodeToString(tx.ID)

		Outputs:
			for outIdx, out := range tx.Outputs {
				if spent[txId] != nil {
					for _, spentOut := range spent[txId] {
						if spentOut == outIdx {
							continue Outputs
						}
					}
				}

				if out.CanBeUnlocked(address) {
					unspent = append(unspent, *tx)
				}
			}

			if !tx.IsCoinbase() {
				for _, in := range tx.Inputs {
					if in.CanUnlock(address) {
						inTxIdx := hex.EncodeToString(in.ID)
						spent[inTxIdx] = append(spent[txId], in.Out)
					}
				}
			}
		}

		if len(block.PrevHash) == 0 {
			break
		}
	}

	return unspent
}

// FindUTXO returns every unspent transaction output that address can unlock.
// Summing their values gives the balance of address.
func (chain *BlockChain) FindUTXO(address string) []TxOutput {
	var Utxos []TxOutput
	unspent := chain.FindUnspentTransactions(address)

	for _, tx := range unspent {
		for _, out := range tx.Outputs {
			if out.CanBeUnlocked(address) {
				Utxos = append(Utxos, out)
			}
		}
	}

	return Utxos
}

// FindSpendableOutputs gathers unspent outputs belonging to address until their
// total reaches amount. It returns that total, which falls short of amount if
// address lacks the funds, together with the indexes of the chosen outputs
// keyed by hex-encoded transaction ID.
func (chain *BlockChain) FindSpendableOutputs(address string, amount int) (int, map[string][]int) {
	unspentOuts := make(map[string][]int)
	unspentTxs := chain.FindUnspentTransactions(address)
	accumulated := 0

Work:
	for _, tx := range unspentTxs {
		txId := hex.EncodeToString(tx.ID)

		for outIdx, out := range tx.Outputs {
			if out.CanBeUnlocked(address) && accumulated < amount {
				accumulated += out.Value
				unspentOuts[txId] = append(unspentOuts[txId], outIdx)
			}

			if accumulated >= amount {
				break Work
			}
		}
	}
	return accumulated, unspentOuts
}
