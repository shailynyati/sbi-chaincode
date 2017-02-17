package main

import (
	//"encoding/json"
	"errors"
	"fmt"
	//"time"

	"github.com/hyperledger/fabric/core/chaincode/shim"
)

//PL ...
/*type PL struct {
	CONSIGNEE_NAME            string
	CONSIGNEE_ADDRESS         string
	PACKING_LIST_NO           string
	DATE                      string
	Rows                      []PLRow
	TOTAL_QUANTITY_MTONS      int
	TOTAL_NET_WEIGHT_KGS      int
	TOTAL_GROSS_WEIGHT_KGS    int
	DELIVERY_TERMS            string
	DOCUMENTARY_CREDIT_NUMBER string
	METHOD_OF_LOADING         string
	CONTAINER_NUMBER          string
	PORT_OF_LOADING           string
	PORT_OF_DISCHARGE         string
	DATE_OF_PRESENTATION      string
}*/

//PLRow ...
/*type PLRow struct {
	DESCRIPTION_OF_GOODS string
	QUANTITY_MTONS       int
	NET_WEIGHT_KGS       int
	GROSS_WEIGHT_KGS     int
}*/

type PL struct{

packingListPDF []byte
	
}

//Init initializes the document smart contract
func (t *PL) Init(stub shim.ChaincodeStubInterface, function string, args []string) ([]byte, error) {
	// Check if table already exists
	_, err := stub.GetTable("PLTable")
	if err == nil {
		// Table already exists; do not recreate
		return nil, nil
	}

	// Create PL Table
	err = stub.CreateTable("PLTable", []*shim.ColumnDefinition{
		&shim.ColumnDefinition{Name: "Type", Type: shim.ColumnDefinition_STRING, Key: true},
		&shim.ColumnDefinition{Name: "UID", Type: shim.ColumnDefinition_STRING, Key: true},
		&shim.ColumnDefinition{Name: "DocPDF", Type: shim.ColumnDefinition_BYTES, Key: false},
		&shim.ColumnDefinition{Name: "Status", Type: shim.ColumnDefinition_STRING, Key: false},
	})
	if err != nil {
		return nil, errors.New("Failed creating PLTable.")
	}

	if err != nil {
		return nil, err
	}

	return nil, nil

}

// isEarlierDate returns true if date1 is earlier than date2, false otherwise
// Assumes that date is presented in 'mm/dd/yyyy' format
/*func (t *PL) isEarlierDate(date1Str string, date2Str string) (bool, error) {
	layout := time_format

	// Parse the dates
	date1, err := time.Parse(layout, date1Str)
	if err != nil {
		return true, errors.New("Incorrect date format for date1. Expecting mm/dd/yyyy; " + date1Str)
	}
	date2, err := time.Parse(layout, date2Str)
	if err != nil {
		return true, errors.New("Incorrect date format for date2. Expecting mm/dd/yyyy; " + date2Str)
	}

	return date1.Before(date2) || date1.Equal(date2), nil
}*/


//SubmitDoc () – Calls ValidateDoc internally and upon success inserts a new row in the table
func (t *PL) SubmitDoc(stub shim.ChaincodeStubInterface, args []string) ([]byte, error) {

	if len(args) != 2 {
		return nil, errors.New("Incorrect number of arguments. Expecting 2.")
	}

	UID := args[0]
	docPDF := []byte(args[1])

	
	// Insert a row
	ok, err := stub.InsertRow("PLTable", shim.Row{
		Columns: []*shim.Column{
			&shim.Column{Value: &shim.Column_String_{String_: "DOC"}},
			&shim.Column{Value: &shim.Column_String_{String_: UID}},
			&shim.Column{Value: &shim.Column_Bytes{Bytes: docPDF}},
			&shim.Column{Value: &shim.Column_String_{String_: "SUBMITTED_BY_EB"}}},
	})

	if !ok && err == nil {
		return nil, errors.New("Document already exists.")
	}

	return nil, err
}

//UpdateStatus () – Updates current document Status. Enforces Status transition logic.
func (t *PL) UpdateStatus(stub shim.ChaincodeStubInterface, args []string) ([]byte, error) {

	if len(args) != 2 {
		return nil, errors.New("Incorrect number of arguments. Expecting 3.")
	}

	UID := args[0]
	newStatus := args[1]

	// Get the row pertaining to this UID
	var columns []shim.Column
	col1 := shim.Column{Value: &shim.Column_String_{String_: "DOC"}}
	columns = append(columns, col1)
	col2 := shim.Column{Value: &shim.Column_String_{String_: UID}}
	columns = append(columns, col2)

	row, err := stub.GetRow("PLTable", columns)
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


	if stateTransitionAllowed == false {
		return nil, errors.New("This state transition is not allowed.")
	}

	//End- Check that the currentStatus to newStatus transition is accurate

	err = stub.DeleteRow(
		"PLTable",
		columns,
	)
	if err != nil {
		return nil, errors.New("Failed deleting row.")
	}

	_, err = stub.InsertRow(
		"PLTable",
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
func (t *PL) GetPDF(stub shim.ChaincodeStubInterface, args []string) ([]byte, error) {

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

	row, err := stub.GetRow("PLTable", columns)
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
func (t *PL) GetStatus(stub shim.ChaincodeStubInterface, args []string) ([]byte, error) {

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

	row, err := stub.GetRow("PLTable", columns)
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
