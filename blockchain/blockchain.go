package blockchain

import (
	"encoding/hex"
	"fmt"
	"os"
	"runtime"

	"github.com/dgraph-io/badger"
)

const (
	path    = "./tmp/blocks"
	dbFile  = "./tmp/blocks/MANIFEST"
	genesis = "First transaction from Genesis"
)

type BlockChain struct {
	LastHash []byte
	Database *badger.DB
}

type BlockChainIterator struct {
	CurrentHash []byte
	Database    *badger.DB
}

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

func DBExists() bool {
	if _, err := os.Stat(dbFile); os.IsNotExist(err) {
		return false
	}
	return true
}

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

func (chain *BlockChain) Iterator() *BlockChainIterator {
	iter := &BlockChainIterator{chain.LastHash, chain.Database}

	return iter
}

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
