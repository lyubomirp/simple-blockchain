package blockchain

import (
	"bytes"
	"crypto/sha256"
	"encoding/gob"
	"encoding/hex"
	"fmt"
)

// Transaction moves token value from its Inputs, which reference outputs of
// earlier transactions, to its Outputs.
type Transaction struct {
	// ID is the hash of the encoded transaction, set by SetID.
	ID      []byte
	Inputs  []TxInput
	Outputs []TxOutput
}

// TxOutput is an amount of tokens locked to the owner of PubKey.
type TxOutput struct {
	// Value is the number of tokens held by the output.
	Value int
	// PubKey identifies the owner. It currently holds the plain address rather
	// than a real public key.
	PubKey string
}

// TxInput references an output of an earlier transaction and carries the proof
// of the right to spend it.
type TxInput struct {
	// ID is the ID of the transaction holding the referenced output.
	ID []byte
	// Out is the index of the referenced output within that transaction.
	Out int
	// Sig proves ownership of the referenced output. It currently holds the
	// plain address rather than a real signature.
	Sig string
}

// CoinbaseTx returns a transaction that mints the mining reward to the address
// to. It has no real input; data records an arbitrary note and defaults to a
// message naming the recipient.
func CoinbaseTx(to, data string) (*Transaction, error) {
	if data == "" {
		data = fmt.Sprintf("Counts to %s", to)
	}

	txin := TxInput{[]byte{}, -1, data}
	txout := TxOutput{100, to}

	tx := Transaction{nil, []TxInput{txin}, []TxOutput{txout}}

	if err := tx.SetID(); err != nil {
		return nil, err
	}

	return &tx, nil
}

// SetID computes the transaction's ID as the hash of its gob encoding.
func (tx *Transaction) SetID() error {
	var encoded bytes.Buffer

	if err := gob.NewEncoder(&encoded).Encode(tx); err != nil {
		return fmt.Errorf("encode transaction: %w", err)
	}

	hash := sha256.Sum256(encoded.Bytes())
	tx.ID = hash[:]

	return nil
}

// IsCoinbase reports whether tx mints new tokens instead of spending existing
// outputs.
func (tx *Transaction) IsCoinbase() bool {
	return len(tx.Inputs) == 1 && len(tx.Inputs[0].ID) == 0 && tx.Inputs[0].Out == -1
}

// CanUnlock reports whether data authorizes spending the output this input
// references.
func (in *TxInput) CanUnlock(data string) bool {
	return in.Sig == data
}

// CanBeUnlocked reports whether data identifies the owner of the output.
func (out *TxOutput) CanBeUnlocked(data string) bool {
	return out.PubKey == data
}

// NewTransaction builds a transaction sending amount from one address to
// another, drawing on chain for spendable outputs and returning any remainder
// to the sender as change. It returns an error wrapping ErrNotEnoughFunds if
// from cannot cover amount.
func NewTransaction(from, to string, amount int, chain *BlockChain) (*Transaction, error) {
	var txIn []TxInput
	var txOut []TxOutput

	acc, validOutputs, err := chain.FindSpendableOutputs(from, amount)
	if err != nil {
		return nil, err
	}

	if acc < amount {
		return nil, fmt.Errorf("%s holds %d of the %d required: %w", from, acc, amount, ErrNotEnoughFunds)
	}

	for txid, outs := range validOutputs {
		txID, err := hex.DecodeString(txid)
		if err != nil {
			return nil, fmt.Errorf("decode transaction ID %q: %w", txid, err)
		}

		for _, out := range outs {
			input := TxInput{txID, out, from}
			txIn = append(txIn, input)
		}
	}

	txOut = append(txOut, TxOutput{amount, to})

	if acc > amount {
		txOut = append(txOut, TxOutput{acc - amount, from})
	}

	tx := Transaction{nil, txIn, txOut}
	if err := tx.SetID(); err != nil {
		return nil, err
	}

	return &tx, nil
}
