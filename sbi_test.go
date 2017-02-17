/*
Copyright IBM Corp. 2016 All Rights Reserved.

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

		 http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

package main

import (
	//"encoding/base64"
	"encoding/json"
	"fmt"
	"net"
	"testing"
	"time"

	"os"
	"path/filepath"

	"github.com/golang/protobuf/proto"
	"github.com/hyperledger/fabric/core/chaincode"
	"github.com/hyperledger/fabric/core/chaincode/shim"
	//"github.com/hyperledger/fabric/core/chaincode/shim/crypto/attr"
	"github.com/hyperledger/fabric/core/container"
	"github.com/hyperledger/fabric/core/crypto"
	"github.com/hyperledger/fabric/core/db"
	"github.com/hyperledger/fabric/core/ledger"
	"github.com/hyperledger/fabric/core/util"
	"github.com/hyperledger/fabric/membersrvc/ca"
	pb "github.com/hyperledger/fabric/protos"
	"github.com/op/go-logging"
	"github.com/spf13/viper"
	"golang.org/x/net/context"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials"
	"google.golang.org/grpc/grpclog"
)

const (
	chaincodeStartupTimeoutDefault int = 5000
)

var (
	testLogger = logging.MustGetLogger("test")

	lis net.Listener

	administrator crypto.Client
	alice         crypto.Client
	bob           crypto.Client

	server *grpc.Server
	aca    *ca.ACA
	eca    *ca.ECA
	tca    *ca.TCA
	tlsca  *ca.TLSCA
)

func TestMain(m *testing.M) {
	removeFolders()
	setup()
	go initMembershipSrvc()

	fmt.Println("Wait for some secs for OBCCA")
	time.Sleep(2 * time.Second)

	go initVP()

	fmt.Println("Wait for some secs for VP")
	time.Sleep(2 * time.Second)

	go initSbiTrade()

	fmt.Println("Wait for some secs for Chaincode")
	time.Sleep(2 * time.Second)

	if err := initClients(); err != nil {
		panic(err)
	}

	fmt.Println("Wait for 5 secs for chaincode to be started")
	time.Sleep(5 * time.Second)

	ret := m.Run()

	closeListenerAndSleep(lis)

	defer removeFolders()
	os.Exit(ret)
}


func deploy(admCert crypto.CertificateHandler) error {
	// Prepare the spec. The metadata includes the role of the users allowed to assign assets
	spec := &pb.ChaincodeSpec{
		Type:                 1,
		ChaincodeID:          &pb.ChaincodeID{Name: "mycc"},
		CtorMsg:              &pb.ChaincodeInput{Args: util.ToChaincodeArgs("init")},
		Metadata:             []byte("issuer"),
		ConfidentialityLevel: pb.ConfidentialityLevel_PUBLIC,
	}

	// First build and get the deployment spec
	var ctx = context.Background()
	chaincodeDeploymentSpec, err := getDeploymentSpec(ctx, spec)
	if err != nil {
		return err
	}

	tid := chaincodeDeploymentSpec.ChaincodeSpec.ChaincodeID.Name

	// Now create the Transactions message and send to Peer.
	transaction, err := administrator.NewChaincodeDeployTransaction(chaincodeDeploymentSpec, tid)
	if err != nil {
		return fmt.Errorf("Error deploying chaincode: %s ", err)
	}

	ledger, err := ledger.GetLedger()
	ledger.BeginTxBatch("1")
	_, _, err = chaincode.Execute(ctx, chaincode.GetChain(chaincode.DefaultChain), transaction)
	if err != nil {
		return fmt.Errorf("Error deploying chaincode: %s", err)
	}
	ledger.CommitTxBatch("1", []*pb.Transaction{transaction}, nil, nil)

	return err
}


func setup() {
	// Conf
	viper.SetConfigName("sbi") // name of config file (without extension)
	viper.AddConfigPath(".")     // path to look for the config file in
	err := viper.ReadInConfig()  // Find and read the config file
	if err != nil {              // Handle errors reading the config file
		panic(fmt.Errorf("Fatal error config file [%s] \n", err))
	}

	// Logging
	var formatter = logging.MustStringFormatter(
		`%{color}[%{module}] %{shortfunc} [%{shortfile}] -> %{level:.4s} %{id:03x}%{color:reset} %{message}`,
	)
	logging.SetFormatter(formatter)

	logging.SetLevel(logging.DEBUG, "peer")
	logging.SetLevel(logging.DEBUG, "chaincode")
	logging.SetLevel(logging.DEBUG, "cryptochain")

	// Init the crypto layer
	if err := crypto.Init(); err != nil {
		panic(fmt.Errorf("Failed initializing the crypto layer [%s]", err))
	}

	removeFolders()

	// Start db
	db.Start()
}

func initMembershipSrvc() {
	// ca.LogInit seems to have been removed
	//ca.LogInit(ioutil.Discard, os.Stdout, os.Stdout, os.Stderr, os.Stdout)
	ca.CacheConfiguration() // Cache configuration
	aca = ca.NewACA()
	eca = ca.NewECA(aca)
	tca = ca.NewTCA(eca)
	tlsca = ca.NewTLSCA(eca)

	var opts []grpc.ServerOption
	if viper.GetBool("peer.pki.tls.enabled") {
		// TLS configuration
		creds, err := credentials.NewServerTLSFromFile(
			filepath.Join(viper.GetString("server.rootpath"), "tlsca.cert"),
			filepath.Join(viper.GetString("server.rootpath"), "tlsca.priv"),
		)
		if err != nil {
			panic("Failed creating credentials for Membersrvc: " + err.Error())
		}
		opts = []grpc.ServerOption{grpc.Creds(creds)}
	}

	fmt.Printf("open socket...\n")
	sockp, err := net.Listen("tcp", viper.GetString("server.port"))
	if err != nil {
		panic("Cannot open port: " + err.Error())
	}
	fmt.Printf("open socket...done\n")

	server = grpc.NewServer(opts...)

	aca.Start(server)
	eca.Start(server)
	tca.Start(server)
	tlsca.Start(server)

	fmt.Printf("start serving...\n")
	server.Serve(sockp)
}

func initVP() {
	var opts []grpc.ServerOption
	if viper.GetBool("peer.tls.enabled") {
		creds, err := credentials.NewServerTLSFromFile(viper.GetString("peer.tls.cert.file"), viper.GetString("peer.tls.key.file"))
		if err != nil {
			grpclog.Fatalf("Failed to generate credentials %v", err)
		}
		opts = []grpc.ServerOption{grpc.Creds(creds)}
	}
	grpcServer := grpc.NewServer(opts...)

	//lis, err := net.Listen("tcp", viper.GetString("peer.address"))

	//use a different address than what we usually use for "peer"
	//we override the peerAddress set in chaincode_support.go
	peerAddress := "0.0.0.0:40404"
	var err error
	lis, err = net.Listen("tcp", peerAddress)
	if err != nil {
		return
	}

	getPeerEndpoint := func() (*pb.PeerEndpoint, error) {
		return &pb.PeerEndpoint{ID: &pb.PeerID{Name: "testpeer"}, Address: peerAddress}, nil
	}

	ccStartupTimeout := time.Duration(chaincodeStartupTimeoutDefault) * time.Millisecond
	userRunsCC := true

	// Install security object for peer
	var secHelper crypto.Peer
	if viper.GetBool("security.enabled") {
		enrollID := viper.GetString("security.enrollID")
		enrollSecret := viper.GetString("security.enrollSecret")
		var err error

		if viper.GetBool("peer.validator.enabled") {
			testLogger.Debugf("Registering validator with enroll ID: %s", enrollID)
			if err = crypto.RegisterValidator(enrollID, nil, enrollID, enrollSecret); nil != err {
				panic(err)
			}
			testLogger.Debugf("Initializing validator with enroll ID: %s", enrollID)
			secHelper, err = crypto.InitValidator(enrollID, nil)
			if nil != err {
				panic(err)
			}
		} else {
			testLogger.Debugf("Registering non-validator with enroll ID: %s", enrollID)
			if err = crypto.RegisterPeer(enrollID, nil, enrollID, enrollSecret); nil != err {
				panic(err)
			}
			testLogger.Debugf("Initializing non-validator with enroll ID: %s", enrollID)
			secHelper, err = crypto.InitPeer(enrollID, nil)
			if nil != err {
				panic(err)
			}
		}
	}

	pb.RegisterChaincodeSupportServer(grpcServer,
		chaincode.NewChaincodeSupport(chaincode.DefaultChain, getPeerEndpoint, userRunsCC,
			ccStartupTimeout, secHelper))

	grpcServer.Serve(lis)
}

func initSbiTrade() {
	err := shim.Start(new(SBI))
	if err != nil {
		panic(err)
	}
}

func initClients() error {
	// Administrator
	if err := crypto.RegisterClient("admin", nil, "admin", "6avZQLwcUe9b"); err != nil {
		return err
	}
	var err error
	administrator, err = crypto.InitClient("admin", nil)
	if err != nil {
		return err
	}

	// Alice
	if err := crypto.RegisterClient("alice", nil, "alice", "NPKYL39uKbkj"); err != nil {
		return err
	}
	alice, err = crypto.InitClient("alice", nil)
	if err != nil {
		return err
	}

	// Bob
	if err := crypto.RegisterClient("bob", nil, "bob", "DRJ23pEQl16a"); err != nil {
		return err
	}
	bob, err = crypto.InitClient("bob", nil)
	if err != nil {
		return err
	}

	return nil
}

func closeListenerAndSleep(l net.Listener) {
	l.Close()
	time.Sleep(2 * time.Second)
}

func getDeploymentSpec(context context.Context, spec *pb.ChaincodeSpec) (*pb.ChaincodeDeploymentSpec, error) {
	fmt.Printf("getting deployment spec for chaincode spec: %v\n", spec)
	var codePackageBytes []byte
	//if we have a name, we don't need to deploy (we are in userRunsCC mode)
	if spec.ChaincodeID.Name == "" {
		var err error
		codePackageBytes, err = container.GetChaincodePackageBytes(spec)
		if err != nil {
			return nil, err
		}
	}
	chaincodeDeploymentSpec := &pb.ChaincodeDeploymentSpec{ChaincodeSpec: spec, CodePackage: codePackageBytes}
	return chaincodeDeploymentSpec, nil
}

func removeFolders() {
	fmt.Println("-------------------------")
	if err := os.RemoveAll(viper.GetString("peer.fileSystemPath")); err != nil {
		fmt.Printf("Failed removing [%s] [%s]\n", "hyperledger", err)
	}
}


func TestCC(t *testing.T) {

	poJSON := []byte(`{
                "Sender":"ABNASCSCAXXX ABN AMRO BANK N.V., SINGAPORE BRANCH SINGAPORE",
                "Receiver":"FTBCUS3CXXX FIFTH THIRD BANK CINCINNATI, OH  US",
                "Tag27":"1/1",
                "Tag40A":"IRREVOCABLE",
                "Tag20":"L960477",
                "Tag31C":"12/07/2012",
                "Tag31D":"120831-USA",
                "Tag50":"ABC Company 21 ANY STREET, SINGAPORE, 659539",
                "Tag59":"KENT COMPANY 52 LOIS LANE, METROPOLIS, IN  48182, USA",
                "Tag32B":"USD10000",
                "Tag39A":"5/5",
                "Tag41A":"FTBCUS3CXXX FIFTH THIRD BANK CINCINNATI, OH  US-Negotiation",
                "Tag42C":"Sight",
                "Tag42D":"ABN AMRO BANK N.V., SINGAPORE BRANCH",
                "Tag43P":"NOT ALLOWED",
                "Tag43T":"ALLOWED",
                "Tag44A":"METROPOLIS, IN",
                "Tag44B":"SINGAPORE",
                "Tag44E":"USA PORT",
                "Tag44F":"SINGAPORE",
                "Tag44C":"03/08/2013",
                "Tag45A":"1 (ONE) UNIT - PORTABLE CHILLER, MODEL : MX-7, SAYR 415 50 AS PER APPLICANT’S P/O NO. 2075 FCA METROPOLIS, IN",
                "Tag46A":" Signed commercial invoice in 1Original(s) plus3 Copy(ies). Packing list in 1Original(s) plus1 Copy(ies).",
                "Tag47A":" All documents are to be sent to Bank of America, N.A. in one registered express airmail or courier unless otherwise specified. Documents issued earlier than L/C issuing date are not acceptable. All documents except draft(s) and commercial invoice(s) must NOT show credit number, name of issuing bank, unit price or invoice value. ",
                "Tag71B":" Confirmation charge is for the account of benificiary. All banking charges outside of Singapore are for the account of beneficiary.",
                "Tag48":"28 days",
                "Tag49":"yes",
                "Tag57D":"MEESNL2A "
                }
                `)


	poNewJSON := []byte(`{
                "Sender":"ABNASCSCAXXX ABN AMRO BANK N.V., SINGAPORE BRANCH SINGAPORE",
                "Receiver":"FTBCUS3CXXX FIFTH THIRD BANK CINCINNATI, OH  US",
                "Tag27":"1/1",
                "Tag40A":"IRREVOCABLE",
                "Tag20":"L960477",
                "Tag31C":"12/07/2012",
                "Tag31D":"120831-USA",
                "Tag50":"ABC Company 21 ANY STREET, SINGAPORE, 659539",
                "Tag59":"KENT COMPANY 52 LOIS LANE, METROPOLIS, IN  48182, USA",
                "Tag32B":"USD40000",
                "Tag39A":"5/5",
                "Tag41A":"FTBCUS3CXXX FIFTH THIRD BANK CINCINNATI, OH  US-Negotiation",
                "Tag42C":"Sight",
                "Tag42D":"ABN AMRO BANK N.V., SINGAPORE BRANCH",
                "Tag43P":"NOT ALLOWED",
                "Tag43T":"ALLOWED",
                "Tag44A":"METROPOLIS, IN",
                "Tag44B":"SINGAPORE",
                "Tag44E":"USA PORT",
                "Tag44F":"SINGAPORE",
                "Tag44C":"03/08/2013",
                "Tag45A":"1 (ONE) UNIT - PORTABLE CHILLER, MODEL : MX-7, SAYR 415 50 AS PER APPLICANT’S P/O NO. 2075 FCA METROPOLIS, IN",
                "Tag46A":" Signed commercial invoice in 1Original(s) plus3 Copy(ies). Packing list in 1Original(s) plus1 Copy(ies).",
                "Tag47A":" All documents are to be sent to Bank of America, N.A. in one registered express airmail or courier unless otherwise specified. Documents issued earlier than L/C issuing date are not acceptable. All documents except draft(s) and commercial invoice(s) must NOT show credit number, name of issuing bank, unit price or invoice value. ",
                "Tag71B":" Confirmation charge is for the account of benificiary. All banking charges outside of Singapore are for the account of beneficiary.",
                "Tag48":"28 days",
                "Tag49":"yes",
                "Tag57D":"MEESNL2A "
                }
                `)
        /*invoiceJSON := []byte(`{
                "PAYER":"A",
                "PAYEE":"B",
                "TAX_REGISTRY_NO":1,
                "INVOICE_CODE":2,
                "INVOICE_NUMBER":3,
                "PRINTING_NO":4,
                "Rows":[
                        {"SERVICE":"A","ITEM":2,"AMOUNT_CHARGED":40,"REMARKS":"A"},
                        {"SERVICE":"B","ITEM":21,"AMOUNT_CHARGED":60,"REMARKS":"A"}
                ],
                "TOTAL_IN_WORDS":"ONE HUNDRED",
                "TOTAL_IN_FIGURES":10000,
                "PRINT_NO":5,
                "ANTI_FORGERY_CODE":"ABCD",
                "DATE_ISSUED":"12/13/2012",
                "DUE_DATE":"12/28/2014",
                "SHIPPING_DATE":"12/15/2012",
                "LC_NUMBER":"L960477",
                "DATE_OF_PRESENTATION":"01/01/2013",
                "CURRENCY":"USD"
                }
                `)

       blJSON := []byte(`{
        "SCAC":"A",
        "BL_NO":101,
        "BOOKING_NO":112,
        "EXPORT_REFERENCES":"B",
        "SVC_CONTRACT":"C",
        "ONWARD_INLAND_ROUTING":"D",
        "SHIPPER_NAME_ADDRESS":"E",
        "CONSIGNEE_NAME_ADDRESS":"F",
        "VESSEL":"G",
        "VOYAGE_NO":32,
        "PORT_OF_LOADING":"H",
        "PORT_OF_DISCHARGE":"I",
        "PLACE_OF_RECEIPT":"J",
        "PLACE_OF_DELIVERY":"K",
        "Rows":[
                {"DESCRIPTION_OF_GOODS":"NUCLEAR_REACTOR","WEIGHT":90,"MEASUREMENT":102}
        ],
        "FREIGHT_AND_CHARGES":4500,
        "RATE":51,
        "UNIT":60,
        "CURRENCY":"USD",
        "PREPAID":"Y",
        "TOTAL_CONTAINERS_RECEIVED_BY_CARRIER":1,
        "CONTAINER_NUMBER":"MSCU 120870-8",
        "PLACE_OF_ISSUE_OF_BL":"SINGAPORE",
        "NUMBER_AND_SEQUENCE_OF_ORIGINAL_BLS":"3",
        "DATE_OF_ISSUE_OF_BL":"12/15/2012",
        "DECLARED_VALUE":9950,
        "SHIPPER_ON_BOARD_DATE":"12/15/2012",
        "SIGNED_BY":"MAERSK",
        "LC_NUMBER":"L960477",
        "DATE_OF_PRESENTATION":"01/01/2013"
}
`)




        plJSON := []byte(`{
        "CONSIGNEE_NAME":"North American Coating and Painting Co. Ltd.",
        "CONSIGNEE_ADDRESS":"Richmond, BC, Canada",
        "PACKING_LIST_NO":"PL-14072014",
        "DATE":"12/15/2012",
        "Rows":[
                {"DESCRIPTION_OF_GOODS":"Pure Polyyester Powder Coating","QUANTITY_MTONS":20,"NET_WEIGHT_KGS":20,"GROSS_WEIGHT_KGS":22}
        ],
        "TOTAL_QUANTITY_MTONS":20,
        "TOTAL_NET_WEIGHT_KGS":20,
        "TOTAL_GROSS_WEIGHT_KGS":22,
        "DELIVERY_TERMS":"CIF Port Metro Vancouver, Canada Incoterms 2010",
        "DOCUMENTARY_CREDIT_NUMBER":"L960477",
        "METHOD_OF_LOADING":"1X40 HQ CNTR (S)",
        "CONTAINER_NUMBER":"MSCU 120870-8",
        "PORT_OF_LOADING":"PORT OF SHANGHAI, CHINA",
        "PORT_OF_DISCHARGE":"PORT METRO VANCOUVER, CANADA",
        "DATE_OF_PRESENTATION":"01/01/2013"
}
`)
*/



        type Status struct {
                Status string
        }
        status := Status{}

       

        type Count struct {
                NumContracts int
        }
        count := Count{}



        // Contract struct
        type Contract struct {
                ContractID string `json:"contractID"`
        }
        //contract := Contract{}

/*
        type Result struct {
                Result string `json:"result"`
        }
        result := Result{}
*/
        // ContractsList struct
        type ContractsList struct {
                Contracts []Contract `json:"contracts"`
        }
        contractsList := ContractsList{}


        // Administrator deploy the chaicode
        adminCert, err := administrator.GetTCertificateHandlerNext("role")
        if err != nil {
                t.Fatal(err)
        }

        if err := deploy(adminCert); err != nil {
                t.Fatal(err)
        }

       /*b, err := validatePO(poJSON)
        err = json.Unmarshal(b, &result)
        if err != nil || result.Result != "Success: The PO passed all validation rules." {
                t.Fatal(err)
        }
        */

        /* WORKFLOW 1: Start */
        // Happy path: submit ed, accept ed


     //This must succeed
		if err = initTrade(adminCert, "1000",poJSON,"I", "E", "IB", "EB", []byte(`ICert`), []byte(`ECert`), []byte(`IBCert`), []byte(`EBCert`)); err!=nil{
        //panic(err)
			t.Fatal(err)
		}

		/*b, err = getPOStatus("1000")
        err = json.Unmarshal(b, &status)
        if err != nil || status.Status != "SUBMITTED_BY_IB" {
                t.Fatal(err)
        }

        // This must succeed
        if err = acceptPO(adminCert, "1000"); err != nil {
                t.Fatal(err)
        }

        // This must succeed
        b, err = getPOStatus("1000")
        err = json.Unmarshal(b, &status)
        if err != nil || status.Status != "ACCEPTED_BY_EB" {
                t.Fatal(err)
        }
		*/
		
        // This must succeed
        if err = submitED(adminCert, "1000", []byte(`BLPDF`), []byte(`INPDF`), []byte(`PLPDF`)); err != nil {
                t.Fatal(err)
        } 

      //This must succeed
		b, err := getEDStatus("1000"); 
        err = json.Unmarshal(b, &status)
        if err !=nil || status.Status != "SUBMITTED_BY_EB" {
        	t.Fatal(err)
        }
     
       // This must succeed
        if err = acceptED(adminCert, "1000"); err != nil {
                t.Fatal(err)
        }

        //This must succeed
	    b, err = getEDStatus("1000"); 
	    err = json.Unmarshal(b, &status)

	    if err !=nil || status.Status != "ACCEPTED_BY_IB"{
        	t.Fatal(err)
        }
	
		 if err = acceptED(adminCert, "1000"); err != nil {
                t.Fatal(err)
        }

        //This must succeed
	    b, err = getEDStatus("1000"); 
	    err = json.Unmarshal(b, &status)

	    if err !=nil || status.Status != "PAYMENT_INITIATED"{
        	t.Fatal(err)
        }
        
       if err = updatePO(adminCert, "1000",poNewJSON); err != nil {
                t.Fatal(err)
        }
        
        /* WORKFLOW 1: End */



       /* WORKFLOW 2: Start */

       //  sad path submit ed, reject ed, 
	

        //This must succeed
		if err = initTrade(adminCert, "1001",poJSON, "I", "E", "IB", "EB", []byte(`ICert`), []byte(`ECert`), []byte(`IBCert`), []byte(`EBCert`)); err!=nil{
        t.Fatal(err)
		}

		

	  // This must succeed
        if err = submitED(adminCert, "1001", []byte(`BLPDF`), []byte(`INPDF`), []byte(`PLPDF`)); err != nil {
                t.Fatal(err)
        }
       
       //This must succeed
        b, err = getEDStatus("1001"); 
        err = json.Unmarshal(b, &status)
        if err != nil || status.Status != "SUBMITTED_BY_EB" {
        	t.Fatal(err)
        }
        
        // This must succeed
        if err = rejectED(adminCert, "1001"); err != nil {
                t.Fatal(err)
        }

        
	       //This must succeed
	    b, err =getEDStatus("1001"); 
	    err = json.Unmarshal(b, &status)

	    if err !=nil || status.Status != "REJECTED_BY_IB"{
        	t.Fatal(err)
        }

        
       
        /* WORKFLOW 2: End */

        
 

	/* WORKFLOW Last: Start Testing Auditing Services APIs */

	//getNumContracts
	// This must succeed

        
	b, err = getNumContracts()
	err = json.Unmarshal(b, &count)
	if err != nil {
		t.Fatal(err)
	}

	

	//getContractParticipants
	contractParticipants, err := getContractParticipants("1000")
	if err != nil {
		t.Fatal(err)
	}

	//listContracts
	cl, err := listContracts()
	err = json.Unmarshal(cl, &contractsList)
	if err != nil {
		t.Fatal(err)
	}

	//listContractsByRole
	clri, err := listContractsByRole("Importer")
	err = json.Unmarshal(clri, &contractsList)
	if err != nil {
		t.Fatal(err)
	}

	//listContractsByRole
	clre, err := listContractsByRole("Exporter")
	err = json.Unmarshal(clre, &contractsList)
	if err != nil {
		t.Fatal(err)
	}

	//listContractsByRole
	clrib, err := listContractsByRole("ImporterBank")
	err = json.Unmarshal(clrib, &contractsList)
	if err != nil {
		t.Fatal(err)
	}

	//listContractsByRole
	clreb, err := listContractsByRole("ExporterBank")
	err = json.Unmarshal(clreb, &contractsList)
	if err != nil {
		t.Fatal(err)
	}

	

	
	//listEDsByStatus
	eda, err := listEDsByStatus("ACCEPTED_BY_IB")
	err = json.Unmarshal(eda, &contractsList)
	if err != nil {
		t.Fatal(err)
	}

	//listEDsByStatus
	edr, err := listEDsByStatus("REJECTED_BY_IB")
	err = json.Unmarshal(edr, &contractsList)
	if err != nil {
		t.Fatal(err)
	}

	//listEDsByStatus
	eds, err := listEDsByStatus("SUBMITTED_BY_EB")
	err = json.Unmarshal(eds, &contractsList)
	if err != nil {
		t.Fatal(err)
	}

	

	fmt.Println("NumContracts = ", count.NumContracts)
	fmt.Println("Contract Participants = ", string(contractParticipants))
	fmt.Println("List of contracts = ", string(cl))
	fmt.Println("List of contracts by role (exporter) = ", string(clre))
	fmt.Println("List of contracts by role (importer) = ", string(clri))
	fmt.Println("List of contracts by role (importer bank) = ", string(clrib))
	fmt.Println("List of contracts by role (exporter bank) = ", string(clreb))
	//fmt.Println("List of LCs by status (PAYMENT_RECEIVED) = ", string(llcr))
	//fmt.Println("List of LCs by status (PAYMENT_DEFAULTED) = ", string(llcd))
	fmt.Println("List of EDs by status (ACCEPTED_BY_IB) = ", string(eda))
	fmt.Println("List of EDs by status (REJECTED_BY_IB) = ", string(edr))
	fmt.Println("List of EDs by status (SUBMITTED_BY_EB) = ", string(eds))
	//fmt.Println("List of EDs by status (PAYMENT_DUE_FROM_IB_TO_EB) = ", string(edp))

	/* WORKFLOW Last: End */
}

//initTrade

func initTrade(admCert crypto.CertificateHandler, contractID string, POJSON []byte, importerName string, exporterName string, importerBankName string, exporterBankName string, importerCert []byte, exporterCert []byte, importerBankCert []byte, exporterBankCert []byte) error {
        // Get a transaction handler to be used to submit the execute transaction
        // and bind the chaincode access control logic using the binding

        submittingCertHandler, err := administrator.GetTCertificateHandlerNext()
        if err != nil {
                return err
        }
        txHandler, err := submittingCertHandler.GetTransactionHandler()
        if err != nil {
                return err
        }
        binding, err := txHandler.GetBinding()
        if err != nil {
                return err
        }

        //chaincodeInput := &pb.ChaincodeInput{Function: "submitLC", Args: []string{contractID, string(LCJSON), importerName, exporterName, importerBankName, exporterBankName, string(importerCert), string(exporterCert), string(importerBankCert), string(exporterBankCert)}}
        chaincodeInput := &pb.ChaincodeInput{Args: [][]byte{[]byte("initTrade"), []byte(contractID),POJSON,[]byte(importerName), []byte(exporterName), []byte(importerBankName), []byte(exporterBankName), importerCert, exporterCert, importerBankCert, exporterBankCert}}
        chaincodeInputRaw, err := proto.Marshal(chaincodeInput)
        if err != nil {
                return err
        }

        // Access control. Administrator signs chaincodeInputRaw || binding to confirm his identity
        sigma, err := admCert.Sign(append(chaincodeInputRaw, binding...))
        if err != nil {
                return err
        }

        // Prepare spec and submit
        spec := &pb.ChaincodeSpec{
                Type:                 1,
                ChaincodeID:          &pb.ChaincodeID{Name: "mycc"},
                CtorMsg:              chaincodeInput,
                Metadata:             sigma, // Proof of identity
                ConfidentialityLevel: pb.ConfidentialityLevel_PUBLIC,
        }

        var ctx = context.Background()
        chaincodeInvocationSpec := &pb.ChaincodeInvocationSpec{ChaincodeSpec: spec}

        tid := chaincodeInvocationSpec.ChaincodeSpec.ChaincodeID.Name

        // Now create the Transactions message and send to Peer.
        transaction, err := txHandler.NewChaincodeExecute(chaincodeInvocationSpec, tid)
        if err != nil {
                return fmt.Errorf("Error deploying chaincode: %s ", err)
        }

        ledger, err := ledger.GetLedger()
        ledger.BeginTxBatch("1")
        _, _, err = chaincode.Execute(ctx, chaincode.GetChain(chaincode.DefaultChain), transaction)
        if err != nil {
                return fmt.Errorf("Error deploying chaincode: %s", err)
        }
        ledger.CommitTxBatch("1", []*pb.Transaction{transaction}, nil, nil)

        return err
}


//submitED
func submitED(admCert crypto.CertificateHandler, contractID string, BLPDF []byte, invoicePDF []byte, packingListPDF []byte) error {
        // Get a transaction handler to be used to submit the execute transaction
        // and bind the chaincode access control logic using the binding
        submittingCertHandler, err := administrator.GetTCertificateHandlerNext()
        if err != nil {
                return err
        }
        txHandler, err := submittingCertHandler.GetTransactionHandler()
        if err != nil {
                return err
        }
        binding, err := txHandler.GetBinding()
        if err != nil {
                return err
        }

        //chaincodeInput := &pb.ChaincodeInput{Function: "submitED", Args: []string{contractID, string(BLPDF), string(invoicePDF), string(packingListPDF), string(BLJSON), string(invoiceJSON), string(packingListJSON)}}
        chaincodeInput := &pb.ChaincodeInput{Args: [][]byte{[]byte("submitED"), []byte(contractID), BLPDF, invoicePDF, packingListPDF}}
        chaincodeInputRaw, err := proto.Marshal(chaincodeInput)
        if err != nil {
                return err
        }

        // Access control. Administrator signs chaincodeInputRaw || binding to confirm his identity
        sigma, err := admCert.Sign(append(chaincodeInputRaw, binding...))
        if err != nil {
                return err
        }

        // Prepare spec and submit
        spec := &pb.ChaincodeSpec{
                Type:                 1,
                ChaincodeID:          &pb.ChaincodeID{Name: "mycc"},
                CtorMsg:              chaincodeInput,
                Metadata:             sigma, // Proof of identity
                ConfidentialityLevel: pb.ConfidentialityLevel_PUBLIC,
        }

        var ctx = context.Background()
        chaincodeInvocationSpec := &pb.ChaincodeInvocationSpec{ChaincodeSpec: spec}

        tid := chaincodeInvocationSpec.ChaincodeSpec.ChaincodeID.Name

        // Now create the Transactions message and send to Peer.
        transaction, err := txHandler.NewChaincodeExecute(chaincodeInvocationSpec, tid)
        if err != nil {
                return fmt.Errorf("Error deploying chaincode: %s ", err)
        }

        ledger, err := ledger.GetLedger()
        ledger.BeginTxBatch("1")
        _, _, err = chaincode.Execute(ctx, chaincode.GetChain(chaincode.DefaultChain), transaction)
        if err != nil {
                return fmt.Errorf("Error deploying chaincode: %s", err)
        }
        ledger.CommitTxBatch("1", []*pb.Transaction{transaction}, nil, nil)

        return err
}

//getEDStatus
func getEDStatus(contractID string) ([]byte, error) {
        //chaincodeInput := &pb.ChaincodeInput{Function: "getEDStatus", Args: []string{contractID}}
        chaincodeInput := &pb.ChaincodeInput{Args: [][]byte{[]byte("getEDStatus"), []byte(contractID)}}

        // Prepare spec and submit
        spec := &pb.ChaincodeSpec{
                Type:                 1,
                ChaincodeID:          &pb.ChaincodeID{Name: "mycc"},
                CtorMsg:              chaincodeInput,
                ConfidentialityLevel: pb.ConfidentialityLevel_PUBLIC,
        }

        var ctx = context.Background()
        chaincodeInvocationSpec := &pb.ChaincodeInvocationSpec{ChaincodeSpec: spec}

        tid := chaincodeInvocationSpec.ChaincodeSpec.ChaincodeID.Name

        // Now create the Transactions message and send to Peer.
        transaction, err := administrator.NewChaincodeQuery(chaincodeInvocationSpec, tid)
        if err != nil {
                return nil, fmt.Errorf("Error deploying chaincode: %s ", err)
        }

        ledger, err := ledger.GetLedger()
        ledger.BeginTxBatch("1")
        result, _, err := chaincode.Execute(ctx, chaincode.GetChain(chaincode.DefaultChain), transaction)
        if err != nil {
                return nil, fmt.Errorf("Error deploying chaincode: %s", err)
        }
        ledger.CommitTxBatch("1", []*pb.Transaction{transaction}, nil, nil)

        return result, err
}

//getED
func getED(contractID string, docType string, docFormat string) ([]byte, error) {
        //chaincodeInput := &pb.ChaincodeInput{Function: "getED", Args: []string{contractID, docType, docFormat}}
        chaincodeInput := &pb.ChaincodeInput{Args: [][]byte{[]byte("getED"), []byte(contractID), []byte(docType), []byte(docFormat)}}

        // Prepare spec and submit
        spec := &pb.ChaincodeSpec{
                Type:                 1,
                ChaincodeID:          &pb.ChaincodeID{Name: "mycc"},
                CtorMsg:              chaincodeInput,
                ConfidentialityLevel: pb.ConfidentialityLevel_PUBLIC,
        }

        var ctx = context.Background()
        chaincodeInvocationSpec := &pb.ChaincodeInvocationSpec{ChaincodeSpec: spec}

        tid := chaincodeInvocationSpec.ChaincodeSpec.ChaincodeID.Name

        // Now create the Transactions message and send to Peer.
        transaction, err := administrator.NewChaincodeQuery(chaincodeInvocationSpec, tid)
        if err != nil {
                return nil, fmt.Errorf("Error deploying chaincode: %s ", err)
        }

        ledger, err := ledger.GetLedger()
        ledger.BeginTxBatch("1")
        result, _, err := chaincode.Execute(ctx, chaincode.GetChain(chaincode.DefaultChain), transaction)
        if err != nil {
                return nil, fmt.Errorf("Error deploying chaincode: %s", err)
        }
        ledger.CommitTxBatch("1", []*pb.Transaction{transaction}, nil, nil)

        return result, err
}

func updatePO(admCert crypto.CertificateHandler, contractID string, POJSON []byte) error {
        // Get a transaction handler to be used to submit the execute transaction
        // and bind the chaincode access control logic using the binding
        submittingCertHandler, err := administrator.GetTCertificateHandlerNext()
        if err != nil {
                return err
        }
        txHandler, err := submittingCertHandler.GetTransactionHandler()
        if err != nil {
                return err
        }
        binding, err := txHandler.GetBinding()
        if err != nil {
                return err
        }

        //chaincodeInput := &pb.ChaincodeInput{Function: "submitED", Args: []string{contractID, string(BLPDF), string(invoicePDF), string(packingListPDF), string(BLJSON), string(invoiceJSON), string(packingListJSON)}}
        chaincodeInput := &pb.ChaincodeInput{Args: [][]byte{[]byte("updatePO"), []byte(contractID), POJSON}}
        chaincodeInputRaw, err := proto.Marshal(chaincodeInput)
        if err != nil {
                return err
        }

        // Access control. Administrator signs chaincodeInputRaw || binding to confirm his identity
        sigma, err := admCert.Sign(append(chaincodeInputRaw, binding...))
        if err != nil {
                return err
        }

        // Prepare spec and submit
        spec := &pb.ChaincodeSpec{
                Type:                 1,
                ChaincodeID:          &pb.ChaincodeID{Name: "mycc"},
                CtorMsg:              chaincodeInput,
                Metadata:             sigma, // Proof of identity
                ConfidentialityLevel: pb.ConfidentialityLevel_PUBLIC,
        }

        var ctx = context.Background()
        chaincodeInvocationSpec := &pb.ChaincodeInvocationSpec{ChaincodeSpec: spec}

        tid := chaincodeInvocationSpec.ChaincodeSpec.ChaincodeID.Name

        // Now create the Transactions message and send to Peer.
        transaction, err := txHandler.NewChaincodeExecute(chaincodeInvocationSpec, tid)
        if err != nil {
                return fmt.Errorf("Error deploying chaincode: %s ", err)
        }

        ledger, err := ledger.GetLedger()
        ledger.BeginTxBatch("1")
        _, _, err = chaincode.Execute(ctx, chaincode.GetChain(chaincode.DefaultChain), transaction)
        if err != nil {
                return fmt.Errorf("Error deploying chaincode: %s", err)
        }
        ledger.CommitTxBatch("1", []*pb.Transaction{transaction}, nil, nil)

        return err
}

/*func paymentReceived(admCert crypto.CertificateHandler, contractID string) error {
        // Get a transaction handler to be used to submit the execute transaction
        // and bind the chaincode access control logic using the binding

        submittingCertHandler, err := administrator.GetTCertificateHandlerNext()
        if err != nil {
                return err
        }
        txHandler, err := submittingCertHandler.GetTransactionHandler()
        if err != nil {
                return err
        }
        binding, err := txHandler.GetBinding()
        if err != nil {
                return err
        }

        //chaincodeInput := &pb.ChaincodeInput{Function: "paymentReceived", Args: []string{contractID}}
        chaincodeInput := &pb.ChaincodeInput{Args: [][]byte{[]byte("paymentReceived"), []byte(contractID)}}
        chaincodeInputRaw, err := proto.Marshal(chaincodeInput)
        if err != nil {
                return err
        }

        // Access control. Administrator signs chaincodeInputRaw || binding to confirm his identity
        sigma, err := admCert.Sign(append(chaincodeInputRaw, binding...))
        if err != nil {
                return err
        }

        // Prepare spec and submit
        spec := &pb.ChaincodeSpec{
                Type:                 1,
                ChaincodeID:          &pb.ChaincodeID{Name: "mycc"},
                CtorMsg:              chaincodeInput,
                Metadata:             sigma, // Proof of identity
                ConfidentialityLevel: pb.ConfidentialityLevel_PUBLIC,
        }

        var ctx = context.Background()
        chaincodeInvocationSpec := &pb.ChaincodeInvocationSpec{ChaincodeSpec: spec}

        tid := chaincodeInvocationSpec.ChaincodeSpec.ChaincodeID.Name

        // Now create the Transactions message and send to Peer.
        transaction, err := txHandler.NewChaincodeExecute(chaincodeInvocationSpec, tid)
        if err != nil {
                return fmt.Errorf("Error deploying chaincode: %s ", err)
        }

        ledger, err := ledger.GetLedger()
        ledger.BeginTxBatch("1")
        _, _, err = chaincode.Execute(ctx, chaincode.GetChain(chaincode.DefaultChain), transaction)
        if err != nil {
                return fmt.Errorf("Error deploying chaincode: %s", err)
        }
        ledger.CommitTxBatch("1", []*pb.Transaction{transaction}, nil, nil)

        return err
}*/


/*
//acceptToPay
func acceptToPay(admCert crypto.CertificateHandler, contractID string) error {
        // Get a transaction handler to be used to submit the execute transaction
        // and bind the chaincode access control logic using the binding

        submittingCertHandler, err := administrator.GetTCertificateHandlerNext()
        if err != nil {
                return err
        }
        txHandler, err := submittingCertHandler.GetTransactionHandler()
        if err != nil {
                return err
        }
        binding, err := txHandler.GetBinding()
        if err != nil {
                return err
        }

        //chaincodeInput := &pb.ChaincodeInput{Function: "acceptToPay", Args: []string{contractID}}
        chaincodeInput := &pb.ChaincodeInput{Args: [][]byte{[]byte("acceptToPay"), []byte(contractID)}}
        chaincodeInputRaw, err := proto.Marshal(chaincodeInput)
        if err != nil {
                return err
        }

        // Access control. Administrator signs chaincodeInputRaw || binding to confirm his identity
        sigma, err := admCert.Sign(append(chaincodeInputRaw, binding...))
        if err != nil {
                return err
        }

        // Prepare spec and submit
        spec := &pb.ChaincodeSpec{
                Type:                 1,
                ChaincodeID:          &pb.ChaincodeID{Name: "mycc"},
                CtorMsg:              chaincodeInput,
                Metadata:             sigma, // Proof of identity
                ConfidentialityLevel: pb.ConfidentialityLevel_PUBLIC,
        }

        var ctx = context.Background()
        chaincodeInvocationSpec := &pb.ChaincodeInvocationSpec{ChaincodeSpec: spec}

        tid := chaincodeInvocationSpec.ChaincodeSpec.ChaincodeID.Name

        // Now create the Transactions message and send to Peer.
        transaction, err := txHandler.NewChaincodeExecute(chaincodeInvocationSpec, tid)
        if err != nil {
                return fmt.Errorf("Error deploying chaincode: %s ", err)
        }

        ledger, err := ledger.GetLedger()
        ledger.BeginTxBatch("1")
        _, _, err = chaincode.Execute(ctx, chaincode.GetChain(chaincode.DefaultChain), transaction)
        if err != nil {
                return fmt.Errorf("Error deploying chaincode: %s", err)
        }
        ledger.CommitTxBatch("1", []*pb.Transaction{transaction}, nil, nil)

        return err
}

//defaultedOnPayment
func defaultedOnPayment(admCert crypto.CertificateHandler, contractID string) error {
        // Get a transaction handler to be used to submit the execute transaction
        // and bind the chaincode access control logic using the binding

        submittingCertHandler, err := administrator.GetTCertificateHandlerNext()
        if err != nil {
                return err
        }
        txHandler, err := submittingCertHandler.GetTransactionHandler()
        if err != nil {
                return err
        }
        binding, err := txHandler.GetBinding()
        if err != nil {
                return err
        }

        //chaincodeInput := &pb.ChaincodeInput{Function: "defaultedOnPayment", Args: []string{contractID}}
        chaincodeInput := &pb.ChaincodeInput{Args: [][]byte{[]byte("defaultedOnPayment"), []byte(contractID)}}
        chaincodeInputRaw, err := proto.Marshal(chaincodeInput)
        if err != nil {
                return err
        }

        // Access control. Administrator signs chaincodeInputRaw || binding to confirm his identity
        sigma, err := admCert.Sign(append(chaincodeInputRaw, binding...))
        if err != nil {
                return err
        }

        // Prepare spec and submit
        spec := &pb.ChaincodeSpec{
                Type:                 1,
                ChaincodeID:          &pb.ChaincodeID{Name: "mycc"},
                CtorMsg:              chaincodeInput,
                Metadata:             sigma, // Proof of identity
                ConfidentialityLevel: pb.ConfidentialityLevel_PUBLIC,
        }

        var ctx = context.Background()
        chaincodeInvocationSpec := &pb.ChaincodeInvocationSpec{ChaincodeSpec: spec}

        tid := chaincodeInvocationSpec.ChaincodeSpec.ChaincodeID.Name

        // Now create the Transactions message and send to Peer.
        transaction, err := txHandler.NewChaincodeExecute(chaincodeInvocationSpec, tid)
        if err != nil {
                return fmt.Errorf("Error deploying chaincode: %s ", err)
        }

        ledger, err := ledger.GetLedger()
        ledger.BeginTxBatch("1")
        _, _, err = chaincode.Execute(ctx, chaincode.GetChain(chaincode.DefaultChain), transaction)
        if err != nil {
                return fmt.Errorf("Error deploying chaincode: %s", err)
        }
        ledger.CommitTxBatch("1", []*pb.Transaction{transaction}, nil, nil)

        return err
}

*/

/*func validatePO(POJSON []byte) ([]byte, error) {
        //chaincodeInput := &pb.ChaincodeInput{Function: "validateLC", Args: []string{string(LCJSON)}}
        chaincodeInput := &pb.ChaincodeInput{Args: [][]byte{[]byte("validatePO"), POJSON}}

        // Prepare spec and submit
        spec := &pb.ChaincodeSpec{
                Type:                 1,
                ChaincodeID:          &pb.ChaincodeID{Name: "mycc"},
                CtorMsg:              chaincodeInput,
                ConfidentialityLevel: pb.ConfidentialityLevel_PUBLIC,
        }

        var ctx = context.Background()
        chaincodeInvocationSpec := &pb.ChaincodeInvocationSpec{ChaincodeSpec: spec}

        tid := chaincodeInvocationSpec.ChaincodeSpec.ChaincodeID.Name

        // Now create the Transactions message and send to Peer.
        transaction, err := administrator.NewChaincodeQuery(chaincodeInvocationSpec, tid)
        if err != nil {
                return nil, fmt.Errorf("Error deploying chaincode: %s ", err)
        }

        ledger, err := ledger.GetLedger()
        ledger.BeginTxBatch("1")
        result, _, err := chaincode.Execute(ctx, chaincode.GetChain(chaincode.DefaultChain), transaction)
        if err != nil {
                return nil, fmt.Errorf("Error deploying chaincode: %s", err)
        }
        ledger.CommitTxBatch("1", []*pb.Transaction{transaction}, nil, nil)

        return result, err
}
*/

/*
func getPOStatus(contractID string) ([]byte, error) {
        //chaincodeInput := &pb.ChaincodeInput{Function: "getLCStatus", Args: []string{contractID}}
        chaincodeInput := &pb.ChaincodeInput{Args: [][]byte{[]byte("getPOStatus"), []byte(contractID)}}

        // Prepare spec and submit
        spec := &pb.ChaincodeSpec{
                Type:                 1,
                ChaincodeID:          &pb.ChaincodeID{Name: "mycc"},
                CtorMsg:              chaincodeInput,
                ConfidentialityLevel: pb.ConfidentialityLevel_PUBLIC,
        }

        var ctx = context.Background()
        chaincodeInvocationSpec := &pb.ChaincodeInvocationSpec{ChaincodeSpec: spec}

        tid := chaincodeInvocationSpec.ChaincodeSpec.ChaincodeID.Name

        // Now create the Transactions message and send to Peer.
        transaction, err := administrator.NewChaincodeQuery(chaincodeInvocationSpec, tid)
        if err != nil {
                return nil, fmt.Errorf("Error deploying chaincode: %s ", err)
        }

        ledger, err := ledger.GetLedger()
        ledger.BeginTxBatch("1")
        result, _, err := chaincode.Execute(ctx, chaincode.GetChain(chaincode.DefaultChain), transaction)
        if err != nil {
                return nil, fmt.Errorf("Error deploying chaincode: %s", err)
        }
        ledger.CommitTxBatch("1", []*pb.Transaction{transaction}, nil, nil)

        return result, err
}

func acceptPO(admCert crypto.CertificateHandler, contractID string) error {
        // Get a transaction handler to be used to submit the execute transaction
        // and bind the chaincode access control logic using the binding

        submittingCertHandler, err := administrator.GetTCertificateHandlerNext()
        if err != nil {
                return err
        }
        txHandler, err := submittingCertHandler.GetTransactionHandler()
        if err != nil {
                return err
        }
        binding, err := txHandler.GetBinding()
        if err != nil {
                return err
        }

        //chaincodeInput := &pb.ChaincodeInput{Function: "acceptLC", Args: []string{contractID}}
        chaincodeInput := &pb.ChaincodeInput{Args: [][]byte{[]byte("acceptPO"), []byte(contractID)}}
        chaincodeInputRaw, err := proto.Marshal(chaincodeInput)
        if err != nil {
                return err
        }

        // Access control. Administrator signs chaincodeInputRaw || binding to confirm his identity
        sigma, err := admCert.Sign(append(chaincodeInputRaw, binding...))
        if err != nil {
                return err
        }

        // Prepare spec and submit
        spec := &pb.ChaincodeSpec{
                Type:                 1,
                ChaincodeID:          &pb.ChaincodeID{Name: "mycc"},
                CtorMsg:              chaincodeInput,
                Metadata:             sigma, // Proof of identity
                ConfidentialityLevel: pb.ConfidentialityLevel_PUBLIC,
        }

        var ctx = context.Background()
        chaincodeInvocationSpec := &pb.ChaincodeInvocationSpec{ChaincodeSpec: spec}

        tid := chaincodeInvocationSpec.ChaincodeSpec.ChaincodeID.Name

        // Now create the Transactions message and send to Peer.
        transaction, err := txHandler.NewChaincodeExecute(chaincodeInvocationSpec, tid)
        if err != nil {
                return fmt.Errorf("Error deploying chaincode: %s ", err)
        }

        ledger, err := ledger.GetLedger()
        ledger.BeginTxBatch("1")
        _, _, err = chaincode.Execute(ctx, chaincode.GetChain(chaincode.DefaultChain), transaction)
        if err != nil {
                return fmt.Errorf("Error deploying chaincode: %s", err)
        }
        ledger.CommitTxBatch("1", []*pb.Transaction{transaction}, nil, nil)

        return err
}

*/
//getNumContracts
func getNumContracts() ([]byte, error) {
	//chaincodeInput := &pb.ChaincodeInput{Function: "getNumContracts", Args: []string{}}
	chaincodeInput := &pb.ChaincodeInput{Args: [][]byte{[]byte("getNumContracts")}}

	// Prepare spec and submit
	spec := &pb.ChaincodeSpec{
		Type:                 1,
		ChaincodeID:          &pb.ChaincodeID{Name: "mycc"},
		CtorMsg:              chaincodeInput,
		ConfidentialityLevel: pb.ConfidentialityLevel_PUBLIC,
	}

	var ctx = context.Background()
	chaincodeInvocationSpec := &pb.ChaincodeInvocationSpec{ChaincodeSpec: spec}

	tid := chaincodeInvocationSpec.ChaincodeSpec.ChaincodeID.Name

	// Now create the Transactions message and send to Peer.
	transaction, err := administrator.NewChaincodeQuery(chaincodeInvocationSpec, tid)
	if err != nil {
		return nil, fmt.Errorf("Error deploying chaincode: %s ", err)
	}

	ledger, err := ledger.GetLedger()
	ledger.BeginTxBatch("1")
	result, _, err := chaincode.Execute(ctx, chaincode.GetChain(chaincode.DefaultChain), transaction)
	if err != nil {
		return nil, fmt.Errorf("Error deploying chaincode: %s", err)
	}
	ledger.CommitTxBatch("1", []*pb.Transaction{transaction}, nil, nil)

	return result, err
}

//listContracts
func listContracts() ([]byte, error) {
	//chaincodeInput := &pb.ChaincodeInput{Function: "listContracts", Args: []string{}}
	chaincodeInput := &pb.ChaincodeInput{Args: [][]byte{[]byte("listContracts")}}

	// Prepare spec and submit
	spec := &pb.ChaincodeSpec{
		Type:                 1,
		ChaincodeID:          &pb.ChaincodeID{Name: "mycc"},
		CtorMsg:              chaincodeInput,
		ConfidentialityLevel: pb.ConfidentialityLevel_PUBLIC,
	}

	var ctx = context.Background()
	chaincodeInvocationSpec := &pb.ChaincodeInvocationSpec{ChaincodeSpec: spec}

	tid := chaincodeInvocationSpec.ChaincodeSpec.ChaincodeID.Name

	// Now create the Transactions message and send to Peer.
	transaction, err := administrator.NewChaincodeQuery(chaincodeInvocationSpec, tid)
	if err != nil {
		return nil, fmt.Errorf("Error deploying chaincode: %s ", err)
	}

	ledger, err := ledger.GetLedger()
	ledger.BeginTxBatch("1")
	result, _, err := chaincode.Execute(ctx, chaincode.GetChain(chaincode.DefaultChain), transaction)
	if err != nil {
		return nil, fmt.Errorf("Error deploying chaincode: %s", err)
	}
	ledger.CommitTxBatch("1", []*pb.Transaction{transaction}, nil, nil)

	return result, err
}

//listContractsByRole
func listContractsByRole(role string) ([]byte, error) {
	//chaincodeInput := &pb.ChaincodeInput{Function: "listContractsByRole", Args: []string{role}}
	chaincodeInput := &pb.ChaincodeInput{Args: [][]byte{[]byte("listContractsByRole"), []byte(role)}}

	// Prepare spec and submit
	spec := &pb.ChaincodeSpec{
		Type:                 1,
		ChaincodeID:          &pb.ChaincodeID{Name: "mycc"},
		CtorMsg:              chaincodeInput,
		ConfidentialityLevel: pb.ConfidentialityLevel_PUBLIC,
	}

	var ctx = context.Background()
	chaincodeInvocationSpec := &pb.ChaincodeInvocationSpec{ChaincodeSpec: spec}

	tid := chaincodeInvocationSpec.ChaincodeSpec.ChaincodeID.Name

	// Now create the Transactions message and send to Peer.
	transaction, err := administrator.NewChaincodeQuery(chaincodeInvocationSpec, tid)
	if err != nil {
		return nil, fmt.Errorf("Error deploying chaincode: %s ", err)
	}

	ledger, err := ledger.GetLedger()
	ledger.BeginTxBatch("1")
	result, _, err := chaincode.Execute(ctx, chaincode.GetChain(chaincode.DefaultChain), transaction)
	if err != nil {
		return nil, fmt.Errorf("Error deploying chaincode: %s", err)
	}
	ledger.CommitTxBatch("1", []*pb.Transaction{transaction}, nil, nil)

	return result, err
}

//acceptED
func acceptED(admCert crypto.CertificateHandler, contractID string) error {
        // Get a transaction handler to be used to submit the execute transaction
        // and bind the chaincode access control logic using the binding

        submittingCertHandler, err := administrator.GetTCertificateHandlerNext()
        if err != nil {
                return err
        }
        txHandler, err := submittingCertHandler.GetTransactionHandler()
        if err != nil {
                return err
        }
        binding, err := txHandler.GetBinding()
        if err != nil {
                return err
        }

        //chaincodeInput := &pb.ChaincodeInput{Function: "acceptED", Args: []string{contractID}}
        chaincodeInput := &pb.ChaincodeInput{Args: [][]byte{[]byte("acceptED"), []byte(contractID)}}
        chaincodeInputRaw, err := proto.Marshal(chaincodeInput)
        if err != nil {
                return err
        }

        // Access control. Administrator signs chaincodeInputRaw || binding to confirm his identity
        sigma, err := admCert.Sign(append(chaincodeInputRaw, binding...))
        if err != nil {
                return err
        }

        // Prepare spec and submit
        spec := &pb.ChaincodeSpec{
                Type:                 1,
                ChaincodeID:          &pb.ChaincodeID{Name: "mycc"},
                CtorMsg:              chaincodeInput,
                Metadata:             sigma, // Proof of identity
                ConfidentialityLevel: pb.ConfidentialityLevel_PUBLIC,
        }

        var ctx = context.Background()
        chaincodeInvocationSpec := &pb.ChaincodeInvocationSpec{ChaincodeSpec: spec}

        tid := chaincodeInvocationSpec.ChaincodeSpec.ChaincodeID.Name

        // Now create the Transactions message and send to Peer.
        transaction, err := txHandler.NewChaincodeExecute(chaincodeInvocationSpec, tid)
        if err != nil {
                return fmt.Errorf("Error deploying chaincode: %s ", err)
        }

        ledger, err := ledger.GetLedger()
        ledger.BeginTxBatch("1")
        _, _, err = chaincode.Execute(ctx, chaincode.GetChain(chaincode.DefaultChain), transaction)
        if err != nil {
                return fmt.Errorf("Error deploying chaincode: %s", err)
        }
        ledger.CommitTxBatch("1", []*pb.Transaction{transaction}, nil, nil)

        return err
}

//rejectED
func rejectED(admCert crypto.CertificateHandler, contractID string) error {
	// Get a transaction handler to be used to submit the execute transaction
	// and bind the chaincode access control logic using the binding

	submittingCertHandler, err := administrator.GetTCertificateHandlerNext()
	if err != nil {
		return err
	}
	txHandler, err := submittingCertHandler.GetTransactionHandler()
	if err != nil {
		return err
	}
	binding, err := txHandler.GetBinding()
	if err != nil {
		return err
	}

	//chaincodeInput := &pb.ChaincodeInput{Function: "rejectED", Args: []string{contractID}}
	chaincodeInput := &pb.ChaincodeInput{Args: [][]byte{[]byte("rejectED"), []byte(contractID)}}

	chaincodeInputRaw, err := proto.Marshal(chaincodeInput)
	if err != nil {
		return err
	}

	// Access control. Administrator signs chaincodeInputRaw || binding to confirm his identity
	sigma, err := admCert.Sign(append(chaincodeInputRaw, binding...))
	if err != nil {
		return err
	}

	// Prepare spec and submit
	spec := &pb.ChaincodeSpec{
		Type:                 1,
		ChaincodeID:          &pb.ChaincodeID{Name: "mycc"},
		CtorMsg:              chaincodeInput,
		Metadata:             sigma, // Proof of identity
		ConfidentialityLevel: pb.ConfidentialityLevel_PUBLIC,
	}

	var ctx = context.Background()
	chaincodeInvocationSpec := &pb.ChaincodeInvocationSpec{ChaincodeSpec: spec}

	tid := chaincodeInvocationSpec.ChaincodeSpec.ChaincodeID.Name

	// Now create the Transactions message and send to Peer.
	transaction, err := txHandler.NewChaincodeExecute(chaincodeInvocationSpec, tid)
	if err != nil {
		return fmt.Errorf("Error deploying chaincode: %s ", err)
	}

	ledger, err := ledger.GetLedger()
	ledger.BeginTxBatch("1")
	_, _, err = chaincode.Execute(ctx, chaincode.GetChain(chaincode.DefaultChain), transaction)
	if err != nil {
		return fmt.Errorf("Error deploying chaincode: %s", err)
	}
	ledger.CommitTxBatch("1", []*pb.Transaction{transaction}, nil, nil)

	return err
}

//getContractParticipants
func getContractParticipants(contractID string) ([]byte, error) {
	//chaincodeInput := &pb.ChaincodeInput{Function: "getContractParticipants", Args: []string{contractID}}
	chaincodeInput := &pb.ChaincodeInput{Args: [][]byte{[]byte("getContractParticipants"), []byte(contractID)}}

	// Prepare spec and submit
	spec := &pb.ChaincodeSpec{
		Type:                 1,
		ChaincodeID:          &pb.ChaincodeID{Name: "mycc"},
		CtorMsg:              chaincodeInput,
		ConfidentialityLevel: pb.ConfidentialityLevel_PUBLIC,
	}

	var ctx = context.Background()
	chaincodeInvocationSpec := &pb.ChaincodeInvocationSpec{ChaincodeSpec: spec}

	tid := chaincodeInvocationSpec.ChaincodeSpec.ChaincodeID.Name

	// Now create the Transactions message and send to Peer.
	transaction, err := administrator.NewChaincodeQuery(chaincodeInvocationSpec, tid)
	if err != nil {
		return nil, fmt.Errorf("Error deploying chaincode: %s ", err)
	}

	ledger, err := ledger.GetLedger()
	ledger.BeginTxBatch("1")
	result, _, err := chaincode.Execute(ctx, chaincode.GetChain(chaincode.DefaultChain), transaction)
	if err != nil {
		return nil, fmt.Errorf("Error deploying chaincode: %s", err)
	}
	ledger.CommitTxBatch("1", []*pb.Transaction{transaction}, nil, nil)

	return result, err
}

//listEDsByStatus
func listEDsByStatus(status string) ([]byte, error) {
	//chaincodeInput := &pb.ChaincodeInput{Function: "listEDsByStatus", Args: []string{status}}
	chaincodeInput := &pb.ChaincodeInput{Args: [][]byte{[]byte("listEDsByStatus"), []byte(status)}}

	// Prepare spec and submit
	spec := &pb.ChaincodeSpec{
		Type:                 1,
		ChaincodeID:          &pb.ChaincodeID{Name: "mycc"},
		CtorMsg:              chaincodeInput,
		ConfidentialityLevel: pb.ConfidentialityLevel_PUBLIC,
	}

	var ctx = context.Background()
	chaincodeInvocationSpec := &pb.ChaincodeInvocationSpec{ChaincodeSpec: spec}

	tid := chaincodeInvocationSpec.ChaincodeSpec.ChaincodeID.Name

	// Now create the Transactions message and send to Peer.
	transaction, err := administrator.NewChaincodeQuery(chaincodeInvocationSpec, tid)
	if err != nil {
		return nil, fmt.Errorf("Error deploying chaincode: %s ", err)
	}

	ledger, err := ledger.GetLedger()
	ledger.BeginTxBatch("1")
	result, _, err := chaincode.Execute(ctx, chaincode.GetChain(chaincode.DefaultChain), transaction)
	if err != nil {
		return nil, fmt.Errorf("Error deploying chaincode: %s", err)
	}
	ledger.CommitTxBatch("1", []*pb.Transaction{transaction}, nil, nil)

	return result, err
}

