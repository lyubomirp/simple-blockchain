// Package blockchain implements a minimal proof-of-work blockchain with a
// UTXO-based token system, persisted in a local Badger key-value store.
package blockchain

import (
	"encoding/hex"
	"errors"
	"fmt"
	"os"

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

// Sentinel errors returned by this package. Callers can test for them with
// errors.Is.
var (
	// ErrChainExists is returned when a blockchain already exists on disk.
	ErrChainExists = errors.New("blockchain already exists")
	// ErrNoChain is returned when no blockchain exists on disk yet.
	ErrNoChain = errors.New("no existing blockchain found")
	// ErrNotEnoughFunds is returned when an address cannot cover a transfer.
	ErrNotEnoughFunds = errors.New("not enough funds")
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
// mines its genesis block, paying the coinbase reward to address. It returns
// ErrChainExists if a chain is already present.
func InitBlockChain(address string) (*BlockChain, error) {
	if DBExists() {
		return nil, ErrChainExists
	}

	if _, err := os.Stat(path); os.IsNotExist(err) {
		fmt.Printf("Creating %v directory \r\n", path)

		if err := os.MkdirAll(path, os.ModePerm); err != nil {
			return nil, fmt.Errorf("create %s directory: %w", path, err)
		}
	}

	db, err := openDB()
	if err != nil {
		return nil, err
	}

	var lastHash []byte

	err = db.Update(func(txn *badger.Txn) error {
		cbtx, err := CoinbaseTx(address, genesis)
		if err != nil {
			return err
		}

		genesisBlock := Genesis(cbtx)

		fmt.Println("Genesis made")

		data, err := genesisBlock.Serialize()
		if err != nil {
			return err
		}

		if err := txn.Set(genesisBlock.Hash, data); err != nil {
			return fmt.Errorf("store genesis block: %w", err)
		}

		if err := txn.Set([]byte("lh"), genesisBlock.Hash); err != nil {
			return fmt.Errorf("store last hash: %w", err)
		}

		lastHash = genesisBlock.Hash

		return nil
	})
	if err != nil {
		db.Close()
		return nil, err
	}

	return &BlockChain{lastHash, db}, nil
}

// DBExists reports whether a blockchain database already exists on disk.
func DBExists() bool {
	if _, err := os.Stat(dbFile); os.IsNotExist(err) {
		return false
	}
	return true
}

// ContinueBlockChain opens the existing blockchain database and returns a chain
// positioned at its last block. It returns ErrNoChain if no chain is present.
func ContinueBlockChain() (*BlockChain, error) {
	if !DBExists() {
		return nil, ErrNoChain
	}

	db, err := openDB()
	if err != nil {
		return nil, err
	}

	lastHash, err := readLastHash(db)
	if err != nil {
		db.Close()
		return nil, err
	}

	return &BlockChain{lastHash, db}, nil
}

// openDB opens the Badger database in the local database directory, with
// Badger's own logging suppressed.
func openDB() (*badger.DB, error) {
	opts := badger.DefaultOptions(path)
	opts.Logger = nil

	db, err := badger.Open(opts)
	if err != nil {
		return nil, fmt.Errorf("open database at %s: %w", path, err)
	}

	return db, nil
}

// readLastHash returns the hash of the chain's newest block.
func readLastHash(db *badger.DB) ([]byte, error) {
	var lastHash []byte

	err := db.View(func(txn *badger.Txn) error {
		item, err := txn.Get([]byte("lh"))
		if err != nil {
			return fmt.Errorf("read last hash: %w", err)
		}

		lastHash, err = item.ValueCopy(nil)
		if err != nil {
			return fmt.Errorf("copy last hash: %w", err)
		}

		return nil
	})
	if err != nil {
		return nil, err
	}

	return lastHash, nil
}

// AddBlock mines a block containing transactions, appends it to the chain and
// records it as the new last hash.
func (chain *BlockChain) AddBlock(transactions []*Transaction) error {
	lastHash, err := readLastHash(chain.Database)
	if err != nil {
		return err
	}

	newBlock := CreateBlock(transactions, lastHash)

	data, err := newBlock.Serialize()
	if err != nil {
		return err
	}

	err = chain.Database.Update(func(txn *badger.Txn) error {
		if err := txn.Set(newBlock.Hash, data); err != nil {
			return fmt.Errorf("store block: %w", err)
		}

		if err := txn.Set([]byte("lh"), newBlock.Hash); err != nil {
			return fmt.Errorf("store last hash: %w", err)
		}

		return nil
	})
	if err != nil {
		return err
	}

	chain.LastHash = newBlock.Hash

	return nil
}

// Iterator returns an iterator positioned at the chain's last block.
func (chain *BlockChain) Iterator() *BlockChainIterator {
	iter := &BlockChainIterator{chain.LastHash, chain.Database}

	return iter
}

// Next returns the block at the iterator's current position and moves the
// iterator one block closer to genesis. Callers stop once the returned block
// has an empty PrevHash; calling Next after that returns an error.
func (iter *BlockChainIterator) Next() (*Block, error) {
	var encodedBlock []byte

	err := iter.Database.View(func(txn *badger.Txn) error {
		item, err := txn.Get(iter.CurrentHash)
		if err != nil {
			return fmt.Errorf("read block %x: %w", iter.CurrentHash, err)
		}

		encodedBlock, err = item.ValueCopy(nil)
		if err != nil {
			return fmt.Errorf("copy block %x: %w", iter.CurrentHash, err)
		}

		return nil
	})
	if err != nil {
		return nil, err
	}

	block, err := Deserialize(encodedBlock)
	if err != nil {
		return nil, err
	}

	iter.CurrentHash = block.PrevHash

	return block, nil
}

// FindUnspentTransactions returns the transactions that still hold at least one
// output unlockable by address.
func (chain *BlockChain) FindUnspentTransactions(address string) ([]Transaction, error) {
	var unspent []Transaction

	spent := make(map[string][]int)

	iter := chain.Iterator()

	for {
		block, err := iter.Next()
		if err != nil {
			return nil, err
		}

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

	return unspent, nil
}

// FindUTXO returns every unspent transaction output that address can unlock.
// Summing their values gives the balance of address.
func (chain *BlockChain) FindUTXO(address string) ([]TxOutput, error) {
	var Utxos []TxOutput

	unspent, err := chain.FindUnspentTransactions(address)
	if err != nil {
		return nil, err
	}

	for _, tx := range unspent {
		for _, out := range tx.Outputs {
			if out.CanBeUnlocked(address) {
				Utxos = append(Utxos, out)
			}
		}
	}

	return Utxos, nil
}

// FindSpendableOutputs gathers unspent outputs belonging to address until their
// total reaches amount. It returns that total, which falls short of amount if
// address lacks the funds, together with the indexes of the chosen outputs
// keyed by hex-encoded transaction ID.
func (chain *BlockChain) FindSpendableOutputs(address string, amount int) (int, map[string][]int, error) {
	unspentOuts := make(map[string][]int)

	unspentTxs, err := chain.FindUnspentTransactions(address)
	if err != nil {
		return 0, nil, err
	}

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

	return accumulated, unspentOuts, nil
}
