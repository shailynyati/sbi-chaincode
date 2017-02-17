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

// Invoice implements the document smart contract
/*type Invoice struct {
	PAYER                string
	PAYEE                string
	TAX_REGISTRY_NO      int
	INVOICE_CODE         int
	INVOICE_NUMBER       int
	PRINTING_NO          int
	Rows                 []invoiceRow
	TOTAL_IN_WORDS       string
	TOTAL_IN_FIGURES     int
	PRINT_NO             int
	ANTI_FORGERY_CODE    string
	DATE_ISSUED          string
	DUE_DATE             string
	SHIPPING_DATE        string
	LC_NUMBER            string
	DATE_OF_PRESENTATION string
	CURRENCY             string
}

RowInvoice ...
type invoiceRow struct {
	//ID             string //`json:"id" bson:"id"`
	SERVICE        string
	ITEM           int
	AMOUNT_CHARGED int
	REMARKS        string
} */

type Invoice struct{

invoicePDF []byte
}
//Init initializes the document smart contract
func (t *Invoice) Init(stub shim.ChaincodeStubInterface, function string, args []string) ([]byte, error) {
	// Check if table already exists
	_, err := stub.GetTable("invoiceTable")
	if err == nil {
		// Table already exists; do not recreate
		return nil, nil
	}

	// Create L/C Table
	err = stub.CreateTable("invoiceTable", []*shim.ColumnDefinition{
		&shim.ColumnDefinition{Name: "Type", Type: shim.ColumnDefinition_STRING, Key: true},
		&shim.ColumnDefinition{Name: "UID", Type: shim.ColumnDefinition_STRING, Key: true},
		&shim.ColumnDefinition{Name: "DocPDF", Type: shim.ColumnDefinition_BYTES, Key: false},
		&shim.ColumnDefinition{Name: "Status", Type: shim.ColumnDefinition_STRING, Key: false},
	})
	if err != nil {
		return nil, errors.New("Failed creating invoiceTable.")
	}

	return nil, nil

}



//SubmitDoc () – Calls ValidateDoc internally and upon success inserts a new row in the table
func (t *Invoice) SubmitDoc(stub shim.ChaincodeStubInterface, args []string) ([]byte, error) {

	if len(args) != 2 {
		return nil, errors.New("Incorrect number of arguments. Expecting 2.")
	}

	UID := args[0]
	docPDF := []byte(args[1])

	
	// Insert a row
	ok, err := stub.InsertRow("invoiceTable", shim.Row{
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
func (t *Invoice) UpdateStatus(stub shim.ChaincodeStubInterface, args []string) ([]byte, error) {

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

	row, err := stub.GetRow("invoiceTable", columns)
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
		"invoiceTable",
		columns,
	)
	if err != nil {
		return nil, errors.New("Failed deleting row.")
	}

	_, err = stub.InsertRow(
		"invoiceTable",
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
func (t *Invoice) GetPDF(stub shim.ChaincodeStubInterface, args []string) ([]byte, error) {

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

	row, err := stub.GetRow("invoiceTable", columns)
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
func (t *Invoice) GetStatus(stub shim.ChaincodeStubInterface, args []string) ([]byte, error) {

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

	row, err := stub.GetRow("invoiceTable", columns)
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
