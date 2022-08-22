package solar

import (
	"fmt"
	"log"
	"net/url"
	"os"
	"strings"
	"sync"

	"github.com/pkg/errors"
	"github.com/kalycoinproject/solar/contract"
	"github.com/kalycoinproject/solar/deployer"
	"github.com/kalycoinproject/solar/deployer/eth"
	"github.com/kalycoinproject/solar/deployer/kalycoin"
	"github.com/kalycoinproject/solar/varstr"
	kingpin "gopkg.in/alecthomas/kingpin.v2"
)

var (
	app               = kingpin.New("solar", "Solidity smart contract deployment management.")
	kalycoinRPC           = app.Flag("kalycoin_rpc", "RPC provider url").Envar("KLC_RPC").String()
	kalycoinSenderAddress = app.Flag("kalycoin_sender", "(kalycoin) Sender UTXO Address").Envar("KLC_SENDER").String()

	// geth --rpc --rpcapi="eth,personal,miner"
	ethRPC    = app.Flag("eth_rpc", "RPC provider url").Envar("ETH_RPC").String()
	solarEnv  = app.Flag("env", "Environment name").Envar("SOLAR_ENV").Default("development").String()
	solarRepo = app.Flag("repo", "Path of contracts repository").Envar("SOLAR_REPO").String()
	appTasks  = map[string]func() error{}

	solcOptimize   = app.Flag("optimize", "[solc] should Enable bytecode optimizer").Default("true").Bool()
	solcAllowPaths = app.Flag("allow-paths", "[solc] Allow a given path for imports. A list of paths can be supplied by separating them with a comma.").Default("").String()
)

type RPCPlatform int

const (
	RPCKalyCoin     = iota
	RPCEthereum = iota
)

type solarCLI struct {
	depoyer      deployer.Deployer
	deployerOnce sync.Once

	repo     *contract.ContractsRepository
	repoOnce sync.Once

	reporter     *events
	reporterOnce sync.Once
}

var solar = &solarCLI{}

var (
	errorUnspecifiedRPC = errors.New("Please specify RPC url by setting KLC_RPC or ETH_RPC or using flag --kalycoin_rpc or --eth_rpc")
)

func (c *solarCLI) RPCPlatform() RPCPlatform {
	if *kalycoinRPC == "" && *ethRPC == "" {
		log.Fatalln(errorUnspecifiedRPC)
	}

	if *kalycoinRPC != "" {
		return RPCKalyCoin
	}

	return RPCEthereum
}

func (c *solarCLI) Reporter() *events {
	c.reporterOnce.Do(func() {
		c.reporter = &events{
			in: make(chan interface{}),
		}

		go c.reporter.Start()
	})

	return c.reporter
}

func (c *solarCLI) SolcOptions() (*CompilerOptions, error) {
	allowPathsStr := *solcAllowPaths
	if allowPathsStr == "" {
		cwd, err := os.Getwd()
		if err != nil {
			return nil, errors.Wrap(err, "solc options")
		}

		allowPathsStr = cwd
	}

	allowPaths := strings.Split(allowPathsStr, ",")

	return &CompilerOptions{
		NoOptimize: !*solcOptimize,
		AllowPaths: allowPaths,
	}, nil
}

// Open the file `solar.{SOLAR_ENV}.json` as contracts repository
func (c *solarCLI) ContractsRepository() *contract.ContractsRepository {
	c.repoOnce.Do(func() {
		var repoFilePath string
		if *solarRepo != "" {
			repoFilePath = *solarRepo
		} else {
			repoFilePath = fmt.Sprintf("solar.%s.json", *solarEnv)
		}

		repo, err := contract.OpenContractsRepository(repoFilePath)
		if err != nil {
			fmt.Printf("error opening contracts repo file %s: %s\n", repoFilePath, err)
			os.Exit(1)
		}

		c.repo = repo
	})

	return c.repo
}

func (c *solarCLI) KalyCoinRPC() *kalycoin.RPC {
	rpc, err := kalycoin.NewRPC(*kalycoinRPC)
	if err != nil {
		fmt.Println("Invalid KLC RPC URL:", *kalycoinRPC)
		os.Exit(1)
	}

	return rpc
}

// ExpandJSONParams uses variable substitution syntax (e.g. $Foo, ${Foo}) to as placeholder for contract addresses
func (c *solarCLI) ExpandJSONParams(jsonParams string) string {
	repo := c.ContractsRepository()

	return varstr.Expand(jsonParams, func(key string) string {
		contract, found := repo.Get(key)
		if !found {
			panic(errors.Errorf("Invalid address expansion: %s", key))
		}

		return contract.Address.String()
	})
}

func (c *solarCLI) ConfigureBytesOutputFormat() {
	if *ethRPC != "" {
		contract.SetFormatBytesWithPrefix(true)
	}
}

func (c *solarCLI) Deployer() (deployer deployer.Deployer) {
	log := log.New(os.Stderr, "", log.Lshortfile)

	var err error
	var rpcURL *url.URL

	if rawurl := *kalycoinRPC; rawurl != "" {

		rpcURL, err = url.ParseRequestURI(rawurl)
		if err != nil {
			log.Fatalf("Invalid RPC url: %#v", rawurl)
		}
		deployer, err = kalycoin.NewDeployer(rpcURL, c.ContractsRepository(), *kalycoinSenderAddress)
	}

	if rawurl := *ethRPC; rawurl != "" {
		rpcURL, err = url.ParseRequestURI(rawurl)
		if err != nil {
			log.Fatalf("Invalid RPC url: %#v", rawurl)
		}

		deployer, err = eth.NewDeployer(rpcURL, c.ContractsRepository())
	}

	if deployer == nil {
		log.Fatalln(errorUnspecifiedRPC)
	}

	if err != nil {
		log.Fatalf("NewDeployer error %v", err)
	}

	return deployer
}

func Main() {
	cmdName, err := app.Parse(os.Args[1:])
	if err != nil {
		fmt.Println(err)
		os.Exit(1)
	}

	solar.ConfigureBytesOutputFormat()

	task := appTasks[cmdName]
	err = task()
	if err != nil {
		fmt.Println(err)
		os.Exit(1)
	}
}
