package main

import (
	//"encoding/json"
	"errors"
	"fmt"
	//"regexp"
	//"strconv"
	//"strings"
	//"time"

	"github.com/hyperledger/fabric/core/chaincode/shim"
)

// Specify the time format
const time_format = "01/02/2006"

// BL implements the document smart contract
/*type BL struct {
	SCAC                                 string
	BL_NO                                int
	BOOKING_NO                           int
	EXPORT_REFERENCES                    string
	SVC_CONTRACT                         string
	ONWARD_INLAND_ROUTING                string
	SHIPPER_NAME_ADDRESS                 string
	CONSIGNEE_NAME_ADDRESS               string
	VESSEL                               string
	VOYAGE_NO                            int
	PORT_OF_LOADING                      string
	PORT_OF_DISCHARGE                    string
	PLACE_OF_RECEIPT                     string
	PLACE_OF_DELIVERY                    string
	Rows                                 []BLRow
	FREIGHT_AND_CHARGES                  int
	RATE                                 int
	UNIT                                 int
	CURRENCY                             string
	PREPAID                              string
	TOTAL_CONTAINERS_RECEIVED_BY_CARRIER int
	CONTAINER_NUMBER                     string
	PLACE_OF_ISSUE_OF_BL                 string
	NUMBER_AND_SEQUENCE_OF_ORIGINAL_BLS  string
	DATE_OF_ISSUE_OF_BL                  string
	DECLARED_VALUE                       int
	SHIPPER_ON_BOARD_DATE                string
	SIGNED_BY                            string
	LC_NUMBER                            string
	DATE_OF_PRESENTATION                 string
}

Row ...
type BLRow struct {
	DESCRIPTION_OF_GOODS string
	WEIGHT               int
	MEASUREMENT          int
} */

type BL struct{

BLPDF []byte 

}


//Init initializes the document smart contract
func (t *BL) Init(stub shim.ChaincodeStubInterface, function string, args []string) ([]byte, error) {
	// Check if table already exists
	_, err := stub.GetTable("BLTable")
	if err == nil {
		// Table already exists; do not recreate
		return nil, nil
	}

	// Create L/C Table
	err = stub.CreateTable("BLTable", []*shim.ColumnDefinition{
		&shim.ColumnDefinition{Name: "Type", Type: shim.ColumnDefinition_STRING, Key: true},
		&shim.ColumnDefinition{Name: "UID", Type: shim.ColumnDefinition_STRING, Key: true},
		//&shim.ColumnDefinition{Name: "DocJSON", Type: shim.ColumnDefinition_BYTES, Key: false},
		&shim.ColumnDefinition{Name: "DocPDF", Type: shim.ColumnDefinition_BYTES, Key: false},
		&shim.ColumnDefinition{Name: "Status", Type: shim.ColumnDefinition_STRING, Key: false},
	})
	if err != nil {
		return nil, errors.New("Failed creating BLTable.")
	}

	return nil, nil

}


//ValidateDoc () – validates that the document is correct


//SubmitDoc () – Calls ValidateDoc internally and upon success inserts a new row in the table
func (t *BL) SubmitDoc(stub shim.ChaincodeStubInterface, args []string) ([]byte, error) {

	if len(args) != 2 {
		return nil, errors.New("Incorrect number of arguments. Expecting 2.")
	}

	UID := args[0]
	docPDF := []byte(args[1])


	// Insert a row
	ok, err := stub.InsertRow("BLTable", shim.Row{
		Columns: []*shim.Column{
			&shim.Column{Value: &shim.Column_String_{String_: "DOC"}},
			&shim.Column{Value: &shim.Column_String_{String_: UID}},
			&shim.Column{Value: &shim.Column_Bytes{Bytes: docPDF}},
			&shim.Column{Value: &shim.Column_String_{String_: "SUBMITTED_BY_EB"}}},
	})

	if !ok && err == nil {
		return nil, errors.New("Document already exists.")
	}

	if err != nil {
		return nil, err
	}

	return nil, nil
}

//UpdateStatus () – Updates current document Status. Enforces Status transition logic.
func (t *BL) UpdateStatus(stub shim.ChaincodeStubInterface, args []string) ([]byte, error) {

	if len(args) != 2 {
		return nil, errors.New("Incorrect number of arguments. Expecting 2.")
	}

	UID := args[0]
	newStatus := args[1]

	// Get the row pertaining to this UID
	var columns []shim.Column
	col1 := shim.Column{Value: &shim.Column_String_{String_: "DOC"}}
	columns = append(columns, col1)
	col2 := shim.Column{Value: &shim.Column_String_{String_: UID}}
	columns = append(columns, col2)

	row, err := stub.GetRow("BLTable", columns)
	if err != nil {
		return nil, fmt.Errorf("Error: Failed retrieving document with UID %s. Error %s", UID, err.Error())
	}

	// GetRows returns empty message if key does not exist
	if len(row.Columns) == 0 {
		return nil, nil
	}

	
	docPDF := row.Columns[2].GetBytes()
	currStatus := row.Columns[3].GetString_()

	//Start- Check that the currentStatus to newStatus transition is accurate

	stateTransitionAllowed := false

	//SUBMITTED_BY_EB -> ACCEPTED_BY_IB
	//SUBMITTED_BY_EB -> REJECTED_BY_IB

	if currStatus == "SUBMITTED_BY_EB" && newStatus == "ACCEPTED_BY_IB" {
		stateTransitionAllowed = true
	} else if currStatus == "SUBMITTED_BY_EB" && newStatus == "REJECTED_BY_IB" {
		stateTransitionAllowed = true
	} else if currStatus == "ACCEPTED_BY_IB" && newStatus == "PAYMENT_INITIATED" {
		stateTransitionAllowed = true
	} else if currStatus == "PAYMENT_INITIATED" && newStatus == "PAYMENT_INPROGRESS" {
		stateTransitionAllowed = true
	} else if currStatus == "PAYMENT_INPROGRESS" && newStatus == "PAYMENT_COMPLETED" {
		stateTransitionAllowed = true
	}

	fmt.Println("state is to identify  ", stateTransitionAllowed)

	if stateTransitionAllowed == false {
		return nil, errors.New("This state transition is not allowed.")
	}
	

	//End- Check that the currentStatus to newStatus transition is accurate


	err = stub.DeleteRow(
		"BLTable",
		columns,
	)
	if err != nil {
		return nil, errors.New("Failed deleting row.")
	}

	_, err = stub.InsertRow(
		"BLTable",
		shim.Row{
			Columns: []*shim.Column{
				&shim.Column{Value: &shim.Column_String_{String_: "DOC"}},
				&shim.Column{Value: &shim.Column_String_{String_: UID}},
				&shim.Column{Value: &shim.Column_Bytes{Bytes: docPDF}},
				&shim.Column{Value: &shim.Column_String_{String_: newStatus}}},
		})
	if err != nil {
		return nil, errors.New("Failed inserting row.")
	}

	return nil, nil

}



// GetPDF () – returns as JSON a single document w.r.t. the UID
func (t *BL) GetPDF(stub shim.ChaincodeStubInterface, args []string) ([]byte, error) {

	if len(args) != 1 {
		return nil, errors.New("Incorrect number of arguments. Expecting 1.")
	}

	UID := args[0]

	// Get the row pertaining to this UID
	var columns []shim.Column
	col1 := shim.Column{Value: &shim.Column_String_{String_: "DOC"}}
	columns = append(columns, col1)
	col2 := shim.Column{Value: &shim.Column_String_{String_: UID}}
	columns = append(columns, col2)

	row, err := stub.GetRow("BLTable", columns)
	if err != nil {
		jsonResp := "{\"Error\":\"Failed retrieveing document with UID " + UID + ". Error " + err.Error() + ". \"}"
		return nil, errors.New(jsonResp)
	}

	// GetRows returns empty message if key does not exist
	if len(row.Columns) == 0 {
		return nil, nil
	}

	//return row.Columns[3].GetBytes(), nil
	return row.Columns[2].GetBytes(), nil

}

// GetStatus () – returns as JSON the Status w.r.t. the UID
func (t *BL) GetStatus(stub shim.ChaincodeStubInterface, args []string) ([]byte, error) {

	if len(args) != 1 {
		return nil, errors.New("Incorrect number of arguments. Expecting 1.")
	}

	UID := args[0]

	// Get the row pertaining to this UID
	var columns []shim.Column
	col1 := shim.Column{Value: &shim.Column_String_{String_: "DOC"}}
	columns = append(columns, col1)
	col2 := shim.Column{Value: &shim.Column_String_{String_: UID}}
	columns = append(columns, col2)

	row, err := stub.GetRow("BLTable", columns)
	if err != nil {
		return nil, fmt.Errorf("Error: Failed retrieving document with UID %s. Error %s", UID, err.Error())
	}

	// GetRows returns empty message if key does not exist
	if len(row.Columns) == 0 {
		return nil, nil
	}

	//return []byte(row.Columns[4].GetString_()), nil
	return []byte(row.Columns[3].GetString_()), nil
}
