package main

import (
	"encoding/json"
	"errors"
	"fmt"

	"github.com/hyperledger/fabric/core/chaincode/shim"
	"github.com/hyperledger/fabric/core/crypto/primitives"

	logging "github.com/op/go-logging"
)

// Access control flag - perform access control if flag is true
const accessControlFlag bool = false 

var myLogger = logging.MustGetLogger("access_control_helper")

// Contract struct
type Contract struct {
	ContractID string `json:"contractID"`
	ContractStatus string `json:"contractStatus"`
}

// ContractsList struct
type ContractsList struct {
	Contracts []Contract `json:"contracts"`
}

// Participants struct
//type Participants struct {
//	ImporterBankName string `json:"importerBankName"`
//	ExporterBankName string `json:"exporterBankName"`
//}

// Participant struct
type Participant struct {
	ID   string `json:"id"`
	Role string `json:"role"`
}

// ParticipantList struct
type ParticipantList struct {
	Participants []Participant `json:"participants"`
}

// SBI is a high level smart contract 
type SBI struct {
	po 		PO
	bl      BL
	invoice Invoice
	pl      PL
}

// Init initializes the smart contracts
func (t *SBI) Init(stub shim.ChaincodeStubInterface, function string, args []string) ([]byte, error) {

	// Check if table already exists
	_, err := stub.GetTable("BPTable")
	if err == nil {
		// Table already exists; do not recreate
		return nil, nil
	}

	// Create Business Process Table
	err = stub.CreateTable("BPTable", []*shim.ColumnDefinition{
		&shim.ColumnDefinition{Name: "Type", Type: shim.ColumnDefinition_STRING, Key: true},
		&shim.ColumnDefinition{Name: "UID", Type: shim.ColumnDefinition_STRING, Key: true},
		&shim.ColumnDefinition{Name: "Status", Type: shim.ColumnDefinition_STRING, Key: false},
		&shim.ColumnDefinition{Name: "ImporterName", Type: shim.ColumnDefinition_STRING, Key: false},
		&shim.ColumnDefinition{Name: "ExporterName", Type: shim.ColumnDefinition_STRING, Key: false},
		&shim.ColumnDefinition{Name: "ImporterBankName", Type: shim.ColumnDefinition_STRING, Key: false},
		&shim.ColumnDefinition{Name: "ExporterBankName", Type: shim.ColumnDefinition_STRING, Key: false},
		&shim.ColumnDefinition{Name: "ImporterCert", Type: shim.ColumnDefinition_BYTES, Key: false},
		&shim.ColumnDefinition{Name: "ExporterCert", Type: shim.ColumnDefinition_BYTES, Key: false},
		&shim.ColumnDefinition{Name: "ImporterBankCert", Type: shim.ColumnDefinition_BYTES, Key: false},
		&shim.ColumnDefinition{Name: "ExporterBankCert", Type: shim.ColumnDefinition_BYTES, Key: false},
	})
	if err != nil {
		return nil, errors.New("Failed creating BPTable.")
	}

	t.po.Init(stub, function, args)
	t.bl.Init(stub, function, args)
	t.invoice.Init(stub, function, args)
	t.pl.Init(stub, function, args)

	return nil, nil
}

// isCaller is a helper function that verifies the signature of the caller given the certificate to match with
func (t *SBI) isCaller(stub shim.ChaincodeStubInterface, certificate []byte) (bool, error) {
	myLogger.Debugf("Check caller...")
	fmt.Printf("PDD-DBG: Check caller...")

	sigma, err := stub.GetCallerMetadata()
	if err != nil {
		return false, errors.New("Failed getting metadata")
	}
	payload, err := stub.GetPayload()
	if err != nil {
		return false, errors.New("Failed getting payload")
	}
	binding, err := stub.GetBinding()
	if err != nil {
		return false, errors.New("Failed getting binding")
	}

	myLogger.Debugf("passed certificate [% x]", certificate)
	myLogger.Debugf("passed sigma [% x]", sigma)
	myLogger.Debugf("passed payload [% x]", payload)
	myLogger.Debugf("passed binding [% x]", binding)

	fmt.Printf("PDD-DBG: passed certificate [% x]", certificate)
	fmt.Printf("PDD-DBG: passed sigma [% x]", sigma)
	fmt.Printf("PDD-DBG: passed payload [% x]", payload)
	fmt.Printf("PDD-DBG: passed binding [% x]", binding)

	ok, err := stub.VerifySignature(
		certificate,
		sigma,
		append(payload, binding...),
	)
	if err != nil {
		myLogger.Error("Failed checking signature ", err.Error())
		fmt.Printf("PDD-DBG: Failed checking signature %s", err.Error())
		return ok, err
	}
	if !ok {
		myLogger.Error("Invalid signature")
		fmt.Printf("PDD-DBG: Invalid signature")
	}

	//myLogger.Debug("Check caller...Verified!")
	//fmt.Printf("PDD-DBG: Check caller...Verified!")

	return ok, err
}

// isCallerImporter accepts UID as input and checks if the caller is importer Bank
func (t *SBI) isCallerImporter(stub shim.ChaincodeStubInterface, args []string) (bool, error) {
	if len(args) != 1 {
		return false, errors.New("Incorrect number of arguments. Expecting 1.")
	}

	UID := args[0]

	var columns []shim.Column
	col1 := shim.Column{Value: &shim.Column_String_{String_: "BP"}}
	columns = append(columns, col1)
	col2 := shim.Column{Value: &shim.Column_String_{String_: UID}}
	columns = append(columns, col2)

	row, err := stub.GetRow("BPTable", columns)
	if err != nil {
		return false, errors.New("Failed retrieving row with contract ID " + UID + ". Error " + err.Error())
	}
	if len(row.Columns) == 0 {
		return false, errors.New("Failed retrieving row with contract ID " + UID)
	}

	// Get the importer bank's certificate for this contract - 5th column in the table
	certificate := row.Columns[7].GetBytes()
	

	ok, err := t.isCaller(stub, certificate)
	if err != nil {
		return false, errors.New("Failed checking importer's identity")
	}
	if !ok {
		return false, nil
	}

	return true, nil
}

// ExporterBank accepts UID as input and checks if the caller is Exporter Bank
func (t *SBI) isCallerExporter(stub shim.ChaincodeStubInterface, args []string) (bool, error) {
	if len(args) != 1 {
		return false, errors.New("Incorrect number of arguments. Expecting 1.")
	}

	UID := args[0]

	var columns []shim.Column
	col1 := shim.Column{Value: &shim.Column_String_{String_: "BP"}}
	columns = append(columns, col1)
	col2 := shim.Column{Value: &shim.Column_String_{String_: UID}}
	columns = append(columns, col2)

	row, err := stub.GetRow("BPTable", columns)
	if err != nil {
		return false, errors.New("Failed retrieving row with contract ID " + UID + ". Error " + err.Error())
	}
	if len(row.Columns) == 0 {
		return false, errors.New("Failed retrieving row with contract ID " + UID)
	}

	// Get the exporter bank's certificate for this contract - 6th column in the table
	certificate := row.Columns[8].GetBytes()
	

	ok, err := t.isCaller(stub, certificate)
	if err != nil {
		return false, errors.New("Failed checking exporter identity " + err.Error())
	}
	if !ok {
		return false, nil
	}

	return true, nil
}

// isCallerImporterBank accepts UID as input and checks if the caller is importer Bank
func (t *SBI) isCallerImporterBank(stub shim.ChaincodeStubInterface, args []string) (bool, error) {
	if len(args) != 1 {
		return false, errors.New("Incorrect number of arguments. Expecting 1.")
	}

	UID := args[0]

	var columns []shim.Column
	col1 := shim.Column{Value: &shim.Column_String_{String_: "BP"}}
	columns = append(columns, col1)
	col2 := shim.Column{Value: &shim.Column_String_{String_: UID}}
	columns = append(columns, col2)

	row, err := stub.GetRow("BPTable", columns)
	if err != nil {
		return false, errors.New("Failed retrieving row with contract ID " + UID + ". Error " + err.Error())
	}
	if len(row.Columns) == 0 {
		return false, errors.New("Failed retrieving row with contract ID " + UID)
	}

	// Get the importer bank's certificate for this contract - 5th column in the table
	certificate := row.Columns[9].GetBytes()

	ok, err := t.isCaller(stub, certificate)
	if err != nil {
		return false, errors.New("Failed checking importer bank's identity")
	}
	if !ok {
		return false, nil
	}

	return true, nil
}

// ExporterBank accepts UID as input and checks if the caller is Exporter Bank
func (t *SBI) isCallerExporterBank(stub shim.ChaincodeStubInterface, args []string) (bool, error) {
	if len(args) != 1 {
		return false, errors.New("Incorrect number of arguments. Expecting 1.")
	}

	UID := args[0]

	var columns []shim.Column
	col1 := shim.Column{Value: &shim.Column_String_{String_: "BP"}}
	columns = append(columns, col1)
	col2 := shim.Column{Value: &shim.Column_String_{String_: UID}}
	columns = append(columns, col2)

	row, err := stub.GetRow("BPTable", columns)
	if err != nil {
		return false, errors.New("Failed retrieving row with contract ID " + UID + ". Error " + err.Error())
	}
	if len(row.Columns) == 0 {
		return false, errors.New("Failed retrieving row with contract ID " + UID)
	}

	// Get the exporter bank's certificate for this contract - 6th column in the table
	certificate := row.Columns[10].GetBytes()

	ok, err := t.isCaller(stub, certificate)
	if err != nil {
		return false, errors.New("Failed checking exporter bank's identity " + err.Error())
	}
	if !ok {
		return false, nil
	}

	return true, nil
}

// isCallerParticipant accepts UID as input and checks if the caller is Exporter Bank
func (t *SBI) isCallerParticipant(stub shim.ChaincodeStubInterface, args []string) (bool, error) {
	if len(args) != 1 {
		return false, errors.New("Incorrect number of arguments. Expecting 1.")
	}

	UID := args[0]

	var columns []shim.Column
	col1 := shim.Column{Value: &shim.Column_String_{String_: "BP"}}
	columns = append(columns, col1)
	col2 := shim.Column{Value: &shim.Column_String_{String_: UID}}
	columns = append(columns, col2)

	row, err := stub.GetRow("BPTable", columns)
	if err != nil {
		return false, errors.New("Failed retrieving row with contract ID " + UID + ". Error " + err.Error())
	}
	if len(row.Columns) == 0 {
		return false, errors.New("Failed retrieving row with contract ID " + UID)
	}

	// Get certificates
	certificate1 := row.Columns[7].GetBytes()
	certificate2 := row.Columns[8].GetBytes()
	certificate3 := row.Columns[9].GetBytes()
	certificate4 := row.Columns[10].GetBytes()

	ok1, err1 := t.isCaller(stub, certificate1)
	ok2, err2 := t.isCaller(stub, certificate2)
	ok3, err3 := t.isCaller(stub, certificate3)
	ok4, err4 := t.isCaller(stub, certificate4)

	if err1 != nil && err2 != nil && err3 != nil && err4 != nil {
		return false, errors.New(err1.Error() + " " + err2.Error() + " " + err3.Error() + " " + err4.Error())
	}

	if !ok1 && !ok2 && !ok3 && !ok4 {
		return false, nil
	}

	return true, nil
}

// getNumContracts get total number of LC applications. Helper function to generate next contract ID.
func (t *SBI) getNumContracts(stub shim.ChaincodeStubInterface, args []string) ([]byte, error) {
	if len(args) != 0 {
		return nil, errors.New("Incorrect number of arguments. Expecting 0.")
	}

	var columns []shim.Column
	col1 := shim.Column{Value: &shim.Column_String_{String_: "BP"}}
	columns = append(columns, col1)

	contractCounter := 0

	rows, err := stub.GetRows("BPTable", columns)
	if err != nil {
		return nil, fmt.Errorf("Failed to retrieve row")
	}

	for row := range rows {
		if len(row.Columns) != 0 {
			contractCounter++
		}
	}

	type count struct {
		NumContracts int
	}

	var c count
	c.NumContracts = contractCounter

	return json.Marshal(c)
}

// listContracts  lists all the contracts
func (t *SBI) listContracts(stub shim.ChaincodeStubInterface, args []string) ([]byte, error) {
	if len(args) != 0 {
		return nil, errors.New("Incorrect number of arguments. Expecting 0.")
	}

	var allContractsList ContractsList

	// Get the row pertaining to this contractID
	var columns []shim.Column
	col1 := shim.Column{Value: &shim.Column_String_{String_: "BP"}}
	columns = append(columns, col1)

	rows, err := stub.GetRows("BPTable", columns)
	if err != nil {
		return nil, fmt.Errorf("Failed to retrieve row")
	}

	allContractsList.Contracts = make([]Contract, 0)

	for row := range rows {
		if len(row.Columns) == 0 {
			res := make(map[string]string, 0)
			resjson, err := json.Marshal(res)
			return resjson, err
		}

		var nextContract Contract
		nextContract.ContractID = row.Columns[1].GetString_()
		nextContract.ContractStatus = row.Columns[2].GetString_()

		//api change to send contract status start
		b, err := t.bl.GetStatus(stub,[]string{nextContract.ContractID})
		if err != nil {
			return nil, err
		}

		if string(b) != "" {
		nextContract.ContractStatus = string(b)
	     }
		//end of change 
		if accessControlFlag == true {
			res, err := t.isCallerParticipant(stub, []string{nextContract.ContractID})
			if err != nil {
				return nil, err
			}
			if res == true {
				allContractsList.Contracts = append(allContractsList.Contracts, nextContract)
			}
		} else {
			allContractsList.Contracts = append(allContractsList.Contracts, nextContract)
		}

	}

	return json.Marshal(allContractsList)
}

// listContractsByRole  lists all the contracts where the user belongs to the provided role.
func (t *SBI) listContractsByRole(stub shim.ChaincodeStubInterface, args []string) ([]byte, error) {
	if len(args) != 1 {
		return nil, errors.New("Incorrect number of arguments. Expecting 1.")
	}

	var allContractsList ContractsList

	role := args[0]

	if role != "Importer" && role != "Exporter" && role != "ImporterBank" && role != "ExporterBank" {
		return nil, errors.New("Role should be Importer, Exporter, ImporterBank or ExporterBank.")
	}

	// Get the row pertaining to this contractID
	var columns []shim.Column
	col1 := shim.Column{Value: &shim.Column_String_{String_: "BP"}}
	columns = append(columns, col1)

	rows, err := stub.GetRows("BPTable", columns)
	if err != nil {
		return nil, fmt.Errorf("Failed to retrieve row")
	}

	allContractsList.Contracts = make([]Contract, 0)

	for row := range rows {
		if len(row.Columns) == 0 {
			res := make(map[string]string, 0)
			resjson, err := json.Marshal(res)
			return resjson, err
		}

		var nextContract Contract
		nextContract.ContractID = row.Columns[1].GetString_()
		nextContract.ContractStatus = row.Columns[2].GetString_()

		//api change to send contract status start
		b, err := t.bl.GetStatus(stub,[]string{nextContract.ContractID})
		if err != nil {
			return nil, err
		}

		if string(b) != "" {
		nextContract.ContractStatus = string(b)
	     }
		//end of change


		if role == "Importer" && accessControlFlag == true {
			res, err := t.isCallerImporter(stub, []string{nextContract.ContractID})
			if err != nil {
				return nil, err
			}
			if res == true {
				allContractsList.Contracts = append(allContractsList.Contracts, nextContract)
			}

		} else if role == "Exporter" && accessControlFlag == true {
			res, err := t.isCallerExporter(stub, []string{nextContract.ContractID})
			if err != nil {
				return nil, err
			}
			if res == true {
				allContractsList.Contracts = append(allContractsList.Contracts, nextContract)
			}

		} else if role == "ImporterBank" && accessControlFlag == true {
			res, err := t.isCallerImporterBank(stub, []string{nextContract.ContractID})
			if err != nil {
				return nil, err
			}
			if res == true {
				allContractsList.Contracts = append(allContractsList.Contracts, nextContract)
			}

		} else if role == "ExporterBank" && accessControlFlag == true {
			res, err := t.isCallerExporterBank(stub, []string{nextContract.ContractID})
			if err != nil {
				return nil, err
			}
			if res == true {
				allContractsList.Contracts = append(allContractsList.Contracts, nextContract)
			}

		} else {
			allContractsList.Contracts = append(allContractsList.Contracts, nextContract)
		}

	}

	return json.Marshal(allContractsList)
}

/*
//listLCsByStatus  lists all the contracts
func (t *SBI) listPOsByStatus(stub shim.ChaincodeStubInterface, args []string) ([]byte, error) {
	if len(args) != 1 {
		return nil, errors.New("Incorrect number of arguments. Expecting 1.")
	}

	status := args[0]
	var allContractsList ContractsList

	// Get the row pertaining to this contractID
	var columns []shim.Column
	col1 := shim.Column{Value: &shim.Column_String_{String_: "BP"}}
	columns = append(columns, col1)

	rows, err := stub.GetRows("BPTable", columns)
	if err != nil {
		return nil, fmt.Errorf("Failed to retrieve row")
	}

	allContractsList.Contracts = make([]Contract, 0)

	for row := range rows {
		// GetRows returns empty message if key does not exist
		if len(row.Columns) == 0 {
			res := make(map[string]string, 0)
			resjson, err := json.Marshal(res)
			return resjson, err
		}

		var nextContract Contract

		b, err := t.po.GetStatus(stub, []string{row.Columns[1].GetString_()})
		if err != nil {
			return nil, err
		}

		if status == string(b) {
			nextContract.ContractID = row.Columns[1].GetString_()
			//allContractsList.Contracts = append(allContractsList.Contracts, nextContract)
			if accessControlFlag == true {
				res, err := t.isCallerParticipant(stub, []string{nextContract.ContractID})
				if err != nil {
					return nil, err
				}
				if res == true {
					allContractsList.Contracts = append(allContractsList.Contracts, nextContract)
				}
			} else {
				allContractsList.Contracts = append(allContractsList.Contracts, nextContract)
			}

		}

	}

	return json.Marshal(allContractsList)
}
*/

//listEDsByStatus  lists all the contracts
func (t *SBI) listEDsByStatus(stub shim.ChaincodeStubInterface, args []string) ([]byte, error) {
	if len(args) != 1 {
		return nil, errors.New("Incorrect number of arguments. Expecting 1.")
	}

	status := args[0]
	//docType := args[1]

	//if docType != "BL" && docType != "INVOICE" && docType != "PACKINGLIST" {
	//	return nil, errors.New("Document type should be BL or INVOICE or PACKINGLIST.")
	//}

	var allContractsList ContractsList

	// Get the row pertaining to this contractID
	var columns []shim.Column
	col1 := shim.Column{Value: &shim.Column_String_{String_: "BP"}}
	columns = append(columns, col1)

	rows, err := stub.GetRows("BPTable", columns)
	if err != nil {
		return nil, fmt.Errorf("Failed to retrieve row")
	}

	allContractsList.Contracts = make([]Contract, 0)

	for row := range rows {
		// GetRows returns empty message if key does not exist
		if len(row.Columns) == 0 {
			res := make(map[string]string, 0)
			resjson, err := json.Marshal(res)
			return resjson, err
		}

		var nextContract Contract

		//since all export documents are always kept in the same state, it is enough to check against one.
		b, err := t.bl.GetStatus(stub, []string{row.Columns[1].GetString_()})
		if err != nil {
			return nil, err
		}
		if status == string(b) {
			nextContract.ContractID = row.Columns[1].GetString_()
			nextContract.ContractStatus = string(b)

			//allContractsList.Contracts = append(allContractsList.Contracts, nextContract)
			if accessControlFlag == true {
				res, err := t.isCallerParticipant(stub, []string{nextContract.ContractID})
				if err != nil {
					return nil, err
				}
				if res == true {
					allContractsList.Contracts = append(allContractsList.Contracts, nextContract)
				}
			} else {
				allContractsList.Contracts = append(allContractsList.Contracts, nextContract)
			}

		}

	}

	return json.Marshal(allContractsList)
}

// getContractParticipants () – returns as JSON the Status w.r.t. the UID
func (t *SBI) getContractParticipants(stub shim.ChaincodeStubInterface, args []string) ([]byte, error) {

	if len(args) != 1 {
		return nil, errors.New("Incorrect number of arguments. Expecting 1.")
	}

	var participantList ParticipantList
	participantList.Participants = make([]Participant, 0)

	UID := args[0]

	// Get the row pertaining to this UID
	var columns []shim.Column
	col1 := shim.Column{Value: &shim.Column_String_{String_: "BP"}}
	columns = append(columns, col1)
	col2 := shim.Column{Value: &shim.Column_String_{String_: UID}}
	columns = append(columns, col2)

	row, err := stub.GetRow("BPTable", columns)
	if err != nil {
		return nil, fmt.Errorf("Error: Failed retrieving document with UID %s. Error %s", UID, err.Error())
	}

	// GetRows returns empty message if key does not exist
	if len(row.Columns) == 0 {
		return nil, nil
	}

	var participant Participant
	participant.ID = row.Columns[3].GetString_()
	participant.Role = "Importer"
	participantList.Participants = append(participantList.Participants, participant)

	participant.ID = row.Columns[4].GetString_()
	participant.Role = "Exporter"
	participantList.Participants = append(participantList.Participants, participant)

	participant.ID = row.Columns[5].GetString_()
	participant.Role = "ImporterBank"
	participantList.Participants = append(participantList.Participants, participant)

	participant.ID = row.Columns[6].GetString_()
	participant.Role = "ExporterBank"
	participantList.Participants = append(participantList.Participants, participant)

	return json.Marshal(participantList.Participants)
}




// Invoke invokes the chaincode
func (t *SBI) Invoke(stub shim.ChaincodeStubInterface, function string, args []string) ([]byte, error) {

	if function == "initTrade" {
		if len(args) != 10 {
			return nil, fmt.Errorf("Incorrect number of arguments. Expecting 10. Got: %d.", len(args))
		}

		UID := args[0]
		POJSON := args[1]
		importerName := args[2]
		exporterName := args[3]
		importerBankName := args[4]
		exporterBankName := args[5]
		importerCert := []byte(args[6])
		exporterCert := []byte(args[7])
		importerBankCert := []byte(args[8])
		exporterBankCert := []byte(args[9])

		// Insert a row
		ok, err := stub.InsertRow("BPTable", shim.Row{
			Columns: []*shim.Column{
				&shim.Column{Value: &shim.Column_String_{String_: "BP"}},
				&shim.Column{Value: &shim.Column_String_{String_: UID}},
				//&shim.Column{Value: &shim.Column_String_{String_: "Started"}},
				&shim.Column{Value: &shim.Column_String_{String_: "IN_PROGRESS"}},
				&shim.Column{Value: &shim.Column_String_{String_: importerName}},
				&shim.Column{Value: &shim.Column_String_{String_: exporterName}},
				&shim.Column{Value: &shim.Column_String_{String_: importerBankName}},
				&shim.Column{Value: &shim.Column_String_{String_: exporterBankName}},
				&shim.Column{Value: &shim.Column_Bytes{Bytes: importerCert}},
				&shim.Column{Value: &shim.Column_Bytes{Bytes: exporterCert}},
				&shim.Column{Value: &shim.Column_Bytes{Bytes: importerBankCert}},
				&shim.Column{Value: &shim.Column_Bytes{Bytes: exporterBankCert}}},
		})

		if err != nil {
			return nil, err
		}
		if !ok && err == nil {
			return nil, errors.New("Row already exists.")
		}

		return t.po.SubmitDoc(stub, []string{UID, POJSON})
	
	

	/*else if function == "acceptPO" {
		if accessControlFlag == true {
			res, err := t.isCallerExporterBank(stub, []string{args[0]})
			if err != nil {
				return nil, err
			}
			if res == false {
				return nil, errors.New("Access denied.")
			}
		}
		args = append(args, "ACCEPTED_BY_EB")
		return t.po.UpdateStatus(stub, args)
	}else if function == "rejectPO" {
		if accessControlFlag == true {
			res, err := t.isCallerExporterBank(stub, []string{args[0]})
			if err != nil {
				return nil, err
			}
			if res == false {
				return nil, errors.New("Access denied.")
			}
		}
		args = append(args, "REJECTED_BY_EB")
		return t.po.UpdateStatus(stub, args)
	}  */
	} else if function == "updatePO" {
		if len(args) != 2 {
			return nil, fmt.Errorf("Incorrect number of arguments. Expecting 2. Got: %d.", len(args))
		}


		UID := args[0]
		POJSON := args[1]
		
		if accessControlFlag == true {
			res, err := t.isCallerImporterBank(stub, []string{args[0]})

			if err != nil {
				return nil, err
			}
			if res == false {
				return nil, errors.New("Access denied.")
			}
		}
		
		return t.po.UpdatePO(stub, []string{UID, POJSON})

	 } else if function == "submitED" {
		if accessControlFlag == true {
			res, err := t.isCallerExporterBank(stub, []string{args[0]})
			if err != nil {
				return nil, err
			}
			if res == false {
				return nil, errors.New("Access denied.")
			}
		}

		contractID := args[0]
		BLPDF := args[1]
		invoicePDF := args[2]
		packingListPDF := args[3]
		



		//Submit the BL to the ledger
		if  BLPDF != "" {
			_, err := t.bl.SubmitDoc(stub, []string{contractID, BLPDF})
			if err != nil {
				return nil, err
			}
		}

		//Submit the invoice to the ledger
		if invoicePDF != "" {
			_, err := t.invoice.SubmitDoc(stub, []string{contractID, invoicePDF})
			if err != nil {
				return nil, err
			}
		}

		//Submit the packing list to the ledger
		if  packingListPDF != "" {
			_, err := t.pl.SubmitDoc(stub, []string{contractID,packingListPDF})
			if err != nil {
				return nil, err
			}
		}

		
		//If pay on sight is true in letter of credit, do state transition LC:ACCEPTED -> PAYMENT_RECEIVED
		//var lc LC
		//err = json.Unmarshal(lcJSON, &lc)
		//if err != nil {
		//	return nil, err
		//}

		//if lc.Tag42C == "Sight" {
		//	return t.lc.UpdateStatus(stub, []string{contractID, "PAYMENT_RECEIVED"})
		//}

		return nil, nil
	} else if function == "acceptED" {

		if accessControlFlag == true {
			res, err := t.isCallerImporterBank(stub, []string{args[0]})

			if err != nil {
				return nil, err
			}
			if res == false {
				return nil, errors.New("Access denied.")
			}
		}


		//api change to send contract status start
		b, err := t.bl.GetStatus(stub,[]string{args[0]})
		if err != nil {
			return nil, err
		}


		if string(b) == "SUBMITTED_BY_EB" {
			args = append(args, "ACCEPTED_BY_IB")
	     } else if  string(b) == "ACCEPTED_BY_IB" {
			args = append(args, "PAYMENT_INITIATED")
	     }else if string(b) == "PAYMENT_INITIATED" {
			args = append(args, "PAYMENT_INPROGRESS")
	     }else if string(b) == "PAYMENT_INPROGRESS" {
			args = append(args, "PAYMENT_COMPLETED")
	     }

	     


		//args = append(args, "ACCEPTED_BY_IB")

		_, err = t.bl.UpdateStatus(stub, args)
		if err != nil {
			return nil, err
		}
		_, err = t.invoice.UpdateStatus(stub, args)
		if err != nil {
			return nil, err
		}
		_, err = t.pl.UpdateStatus(stub, args)
		if err != nil {
			return nil, err
		}

		return nil, nil
	} else if function == "rejectED" {

		if accessControlFlag == true {

			res, err := t.isCallerImporterBank(stub, []string{args[0]})

			if err != nil {
				return nil, err
			}
			if res == false {
				return nil, errors.New("Access denied.")
			}
		}

		args = append(args, "REJECTED_BY_IB")

		_, err := t.bl.UpdateStatus(stub, args)
		if err != nil {
			return nil, err
		}
		_, err = t.invoice.UpdateStatus(stub, args)
		if err != nil {
			return nil, err
		}
		_, err = t.pl.UpdateStatus(stub, args)
		if err != nil {
			return nil, err
		}

		return nil, nil
	} 

	/*else if function == "acceptToPay" {

		if accessControlFlag == true {
			//res, err := t.isCallerImporterBank(stub, []string{args[0], string(sigma), string(payload), string(binding)})
			res, err := t.isCallerImporterBank(stub, []string{args[0]})

			if err != nil {
				return nil, err
			}
			if res == false {
				return nil, errors.New("Access denied.")
			}
		}

		args = append(args, "PAYMENT_DUE_FROM_IB_TO_EB")

		
		return nil, nil
	} */

	return nil, errors.New("Invalid invoke function name.")
}

// Query callback representing the query of a chaincode
func (t *SBI) Query(stub shim.ChaincodeStubInterface, function string, args []string) ([]byte, error) {
	type Status struct {
		Status string
	}
	status := Status{}

	/*type Result struct {
		Result string `json:"result"`
	}
	result := Result{}
	*/
	/*
		sigma, err := stub.GetCallerMetadata()
		if err != nil {
			return nil, errors.New("Failed getting metadata")
		}
		payload, err := stub.GetPayload()
		if err != nil {
			return nil, errors.New("Failed getting payload")
		}
		binding, err := stub.GetBinding()
		if err != nil {
			return nil, errors.New("Failed getting binding")
		}
	*/

	if   function == "getED" {
		if accessControlFlag == true {
			res, err := t.isCallerParticipant(stub, []string{args[0]})
			if err != nil {
				return nil, err
			}
			if res == false {
				return nil, errors.New("Access denied.")
			}
		}

		if len(args) != 2 {
			return nil, errors.New("Incorrect number of arguments. Expecting 2.")
		}

		contractID := args[0]
		docType := args[1]
		

		if docType != "BL" && docType != "INVOICE" && docType != "PACKINGLIST" {
			return nil, errors.New("Document type should be BL or INVOICE or PACKINGLIST")
		}

		
		
		
		if docType == "BL" {
				return t.bl.GetPDF(stub, []string{contractID})
			} else if docType == "INVOICE" {
				return t.invoice.GetPDF(stub, []string{contractID})
			} else if docType == "PACKINGLIST" {
				return t.pl.GetPDF(stub, []string{contractID})
			}

		

		return nil, nil
	}else if function == "getPO" {

		if accessControlFlag == true {
			res, err := t.isCallerParticipant(stub, []string{args[0]})
			if err != nil {
				return nil, err
			}
			if res == false {
				return nil, errors.New("Access denied.")
			}
		}

		return t.po.GetJSON(stub, args)
	
	/*else if function == "getPOStatus" {

		if accessControlFlag == true {
			res, err := t.isCallerParticipant(stub, []string{args[0]})
			if err != nil {
				return nil, err
			}
			if res == false {
				return nil, errors.New("Access denied.")
			}
		}

		b, err := t.po.GetStatus(stub, args)
		if err != nil {
			return nil, err
		}
		status.Status = string(b)
		return json.Marshal(status)
	}else if function == "validatePO" {

		b, err := t.po.ValidateDoc(stub, args)
		if err != nil {
			return nil, err
		}
		result.Result = string(b)
		return json.Marshal(result)
	} 

	*/ }else if function == "getEDStatus" {
		if accessControlFlag == true {
			res, err := t.isCallerParticipant(stub, []string{args[0]})
			if err != nil {
				return nil, err
			}
			if res == false {
				return nil, errors.New("Access denied.")
			}
		}

		b, err := t.bl.GetStatus(stub, args)
		if err != nil {
			return nil, err
		}
		status.Status = string(b)
		return json.Marshal(status)
	} else if function == "getNumContracts" {

		return t.getNumContracts(stub, args)
	} else if function == "listContracts" {

		return t.listContracts(stub, args)
	} else if function == "listContractsByRole" {

		return t.listContractsByRole(stub, args)
	}  else if function == "listEDsByStatus" {

		return t.listEDsByStatus(stub, args)
	} else if function == "getContractParticipants" {
		if accessControlFlag == true {
			res, err := t.isCallerParticipant(stub, []string{args[0]})
			if err != nil {
				return nil, err
			}
			if res == false {
				return nil, errors.New("Access denied.")
			}
		}

		return t.getContractParticipants(stub, args)
	}

	return nil, errors.New("Invalid query function name.")
}

func main() {
	primitives.SetSecurityLevel("SHA3", 256)
	err := shim.Start(new(SBI))
	if err != nil {
		fmt.Printf("Error starting TF: %s", err)
	}
}
