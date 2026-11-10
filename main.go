package main

import (
	"errors"
	"flag"
	"fmt"
	"os"
	"strconv"

	"github.com/lyubomirp/simple-blockchain/blockchain"
)

// CommandLine dispatches the command-line subcommands and owns no state.
type CommandLine struct{}

func (cli *CommandLine) printUsage() {
	fmt.Println("Usage: ")
	fmt.Println("getbalance -address ADDRESS - get balance for address")
	fmt.Println("createblockchain  -address ADDRESS - creates a blockchain, and the address mines the genesis")
	fmt.Println("send -from FROM -to TO -amount AMOUNT - send token amount from one account to another")
	fmt.Println("printchain - Prints the blocks in the chain")
}

// closeChain closes the chain's database. A failure to close is reported
// through errp unless an error has already been recorded there.
func closeChain(chain *blockchain.BlockChain, errp *error) {
	if err := chain.Database.Close(); err != nil && *errp == nil {
		*errp = fmt.Errorf("close database: %w", err)
	}
}

func (cli *CommandLine) printChain() (err error) {
	chain, err := blockchain.ContinueBlockChain()
	if err != nil {
		return err
	}
	defer closeChain(chain, &err)

	iter := chain.Iterator()

	for {
		block, err := iter.Next()
		if err != nil {
			return err
		}

		fmt.Printf("Prev Hash: %x\n", block.PrevHash)
		fmt.Printf("Hash: %x\n", block.Hash)

		pow := blockchain.NewProof(block)
		fmt.Printf("Pow: %s\n", strconv.FormatBool(pow.Validate()))
		fmt.Println()

		if len(block.PrevHash) == 0 {
			break
		}
	}

	return nil
}

func (cli *CommandLine) createBlockChain(address string) (err error) {
	chain, err := blockchain.InitBlockChain(address)
	if err != nil {
		return err
	}
	defer closeChain(chain, &err)

	fmt.Println("Finished")

	return nil
}

func (cli *CommandLine) getBalance(address string) (err error) {
	chain, err := blockchain.ContinueBlockChain()
	if err != nil {
		return err
	}
	defer closeChain(chain, &err)

	utxos, err := chain.FindUTXO(address)
	if err != nil {
		return err
	}

	balance := 0
	for _, out := range utxos {
		balance += out.Value
	}

	fmt.Printf("Balance of %s: %d\n", address, balance)

	return nil
}

func (cli *CommandLine) send(from, to string, amount int) (err error) {
	chain, err := blockchain.ContinueBlockChain()
	if err != nil {
		return err
	}
	defer closeChain(chain, &err)

	tx, err := blockchain.NewTransaction(from, to, amount, chain)
	if err != nil {
		return err
	}

	if err := chain.AddBlock([]*blockchain.Transaction{tx}); err != nil {
		return err
	}

	fmt.Println("Success")

	return nil
}

func (cli *CommandLine) run() error {
	if len(os.Args) < 2 {
		cli.printUsage()
		return errors.New("no command given")
	}

	getBalanceCmd := flag.NewFlagSet("getbalance", flag.ExitOnError)
	createBlockchainCmd := flag.NewFlagSet("createblockchain", flag.ExitOnError)
	sendCmd := flag.NewFlagSet("send", flag.ExitOnError)
	printChainCmd := flag.NewFlagSet("printchain", flag.ExitOnError)

	getBalanceAddress := getBalanceCmd.String("address", "", "Print the balance of an address")
	createBlockChainAddress := createBlockchainCmd.String("address", "", "Create blockchain and address mines genesis")
	sendFrom := sendCmd.String("from", "", "Source wallet address")
	sendTo := sendCmd.String("to", "", "Destination wallet address")
	sendAmount := sendCmd.Int("amount", 0, "Amount to send")

	switch os.Args[1] {
	case "getbalance":
		if err := getBalanceCmd.Parse(os.Args[2:]); err != nil {
			return err
		}
	case "createblockchain":
		if err := createBlockchainCmd.Parse(os.Args[2:]); err != nil {
			return err
		}
	case "printChain":
		if err := printChainCmd.Parse(os.Args[2:]); err != nil {
			return err
		}
	case "send":
		if err := sendCmd.Parse(os.Args[2:]); err != nil {
			return err
		}
	default:
		return cli.printChain()
	}

	if getBalanceCmd.Parsed() {
		if *getBalanceAddress == "" {
			getBalanceCmd.Usage()
			return errors.New("getbalance: -address is required")
		}

		return cli.getBalance(*getBalanceAddress)
	}

	if createBlockchainCmd.Parsed() {
		if *createBlockChainAddress == "" {
			createBlockchainCmd.Usage()
			return errors.New("createblockchain: -address is required")
		}

		return cli.createBlockChain(*createBlockChainAddress)
	}

	if sendCmd.Parsed() {
		if *sendFrom == "" || *sendTo == "" || *sendAmount == 0 {
			createBlockchainCmd.Usage()
			return errors.New("send: -from, -to and -amount are required")
		}

		return cli.send(*sendFrom, *sendTo, *sendAmount)
	}

	if printChainCmd.Parsed() {
		return cli.printChain()
	}

	return nil
}

func main() {
	cli := CommandLine{}

	if err := cli.run(); err != nil {
		fmt.Fprintln(os.Stderr, "error:", err)
		os.Exit(1)
	}
}
