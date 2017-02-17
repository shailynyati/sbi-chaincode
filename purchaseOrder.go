package main

import (
	//"encoding/json"
	"errors"
	"fmt"

	"github.com/hyperledger/fabric/core/chaincode/shim"
)
type PO struct {

}
/*
//LC struct
type PO struct {
	Sender   string
	Receiver string
	Tag27    string //Sequence of Total
	Tag40A   string //Form of documentary credit
	Tag20    string //Documentary Credit Number
	Tag31C   string //Date of Issue
	//Tag40E   string //Applicable Rules
	Tag31D string //Date and Place of Expiry
	Tag50  string //Applicant
	Tag59  string //Beneficiary - Name & Address
	Tag32B string //Currency Code, Amount
	Tag39A string //Percentage Credit Amount Tolerance
	//Tag39B string //Maximum Credit Amount
	Tag41A string //Available with… by…
	//Tag41D string
	Tag42C string //Drafts at
	Tag42D string //Drawee
	Tag43P string //Partial Shipments
	Tag43T string //Transhipment
	Tag44A string //Place of Taking in Charge/ Dispatch from.../ Place of Receipt
	Tag44B string //Place of Final Destination/ for Transportation to.../ Place of Delivery:
	Tag44E string //Port of Loading/Airport of Departure
	Tag44F string //Port of Discharge/Airport of Destination
	Tag44C string //Latest Date of Shipment
	Tag45A string //Description of Goods &/or Services
	Tag46A string //Documents Required
	Tag47A string //Additional Conditions
	Tag71B string //Charges
	Tag48  string //Period for Presentation
	Tag49  string //Confirmation Instructions
	//Tag53A string //Reimbursing Bank – BIC
	//Tag78  string //Instruction to Paying/Accepting/Negotiating Bank
	Tag57D string //`Advise Through` Bank -Name&Addr
}

*/
//Init initializes the document smart contract
func (t *PO) Init(stub shim.ChaincodeStubInterface, function string, args []string) ([]byte, error) {
	// Check if table already exists
	_, err := stub.GetTable("POTable")
	if err == nil {
		// Table already exists; do not recreate
		return nil, nil
	}

	// Create L/C Table
	err = stub.CreateTable("POTable", []*shim.ColumnDefinition{
		&shim.ColumnDefinition{Name: "Type", Type: shim.ColumnDefinition_STRING, Key: true},
		&shim.ColumnDefinition{Name: "UID", Type: shim.ColumnDefinition_STRING, Key: true},
		&shim.ColumnDefinition{Name: "DocJSON", Type: shim.ColumnDefinition_BYTES, Key: false},
		//&shim.ColumnDefinition{Name: "DocPDF", Type: shim.ColumnDefinition_BYTES, Key: false},
		&shim.ColumnDefinition{Name: "Status", Type: shim.ColumnDefinition_STRING, Key: false},
	})
	if err != nil {
		return nil, errors.New("Failed creating LCTable.")
	}

	return nil, nil

}

//TODO: Make sure that args[0] is a JSON object that maps to appropriate struct

//ValidateDoc () – validates that the document is correct
/*func (t *PO) ValidateDoc(stub shim.ChaincodeStubInterface, args []string) ([]byte, error) {
	if len(args) != 1 {
		return nil, errors.New("Incorrect number of arguments. Expecting 1.")
	}
	docJSON := []byte(args[0])
	var js PO
	err := json.Unmarshal(docJSON, &js)

	if err != nil {
		return []byte("FAILURE"), err
	}

	//All fields in the LC struct defined above should be present in the JSON.
	if js.Sender == "" {
		return []byte("Error: The Sender field is not set."), nil
	} else if js.Receiver == "" {
		return []byte("Error: The Receiver field is not set."), nil
	} else if js.Tag20 == "" {
		return []byte("Error: Tag20 field is not set."), nil
	} else if js.Tag27 == "" {
		return []byte("Error: Tag27 field is not set."), nil
	} else if js.Tag31C == "" {
		return []byte("Error: Tag31C field is not set."), nil
	} else if js.Tag31D == "" {
		return []byte("Error: Tag31D field is not set."), nil
	} else if js.Tag32B == "" {
		return []byte("Error: Tag32B field is not set."), nil
	} else if js.Tag39A == "" {
		return []byte("Error: Tag39A field is not set."), nil
	} else if js.Tag40A == "" {
		return []byte("Error: Tag40A field is not set."), nil
	} else if js.Tag41A == "" {
		return []byte("Error: Tag41A field is not set."), nil
	} else if js.Tag42C == "" {
		return []byte("Error: Tag42C field is not set."), nil
	} else if js.Tag42D == "" {
		return []byte("Error: Tag42D field is not set."), nil
	} else if js.Tag43P == "" {
		return []byte("Error: Tag43P field is not set."), nil
	} else if js.Tag43T == "" {
		return []byte("Error: Tag43T field is not set."), nil
	} else if js.Tag44A == "" {
		return []byte("Error: Tag44A field is not set."), nil
	} else if js.Tag44B == "" {
		return []byte("Error: Tag44B field is not set."), nil
	} else if js.Tag44C == "" {
		return []byte("Error: Tag44C field is not set."), nil
	} else if js.Tag44E == "" {
		return []byte("Error: Tag44E field is not set."), nil
	} else if js.Tag44F == "" {
		return []byte("Error: Tag44F field is not set."), nil
	} else if js.Tag45A == "" {
		return []byte("Error: Tag45A field is not set."), nil
	} else if js.Tag46A == "" {
		return []byte("Error: Tag46A field is not set."), nil
	} else if js.Tag47A == "" {
		return []byte("Error: Tag47A field is not set."), nil
	} else if js.Tag48 == "" {
		return []byte("Error: Tag48 field is not set."), nil
	} else if js.Tag49 == "" {
		return []byte("Error: Tag49 field is not set."), nil
	} else if js.Tag50 == "" {
		return []byte("Error: Tag50 field is not set."), nil
	} else if js.Tag57D == "" {
		return []byte("Error: Tag57D field is not set."), nil
	} else if js.Tag59 == "" {
		return []byte("Error: Tag59 field is not set."), nil
	} else if js.Tag71B == "" {
		return []byte("Error: Tag71B field is not set."), nil
	}

	return []byte("Success: The PO passed all validation rules."), nil
}
*/
//SubmitDoc () – Calls ValidateDoc internally and upon success inserts a new row in the table
func (t *PO) SubmitDoc(stub shim.ChaincodeStubInterface, args []string) ([]byte, error) {

	if len(args) != 2{
		return nil, errors.New("Incorrect number of arguments. Expecting 2.")
	}

	UID := args[0]
	docJSON := []byte(args[1])
	//docPDF := []byte(args[2])

	/*res, err := t.ValidateDoc(stub, []string{string(docJSON)})
	if err != nil {
		return nil, err
	}
	if string(res) == "FAILURE" {
		return nil, errors.New("Document validation failed.")
	}
	*/

	// Insert a row
	ok, err := stub.InsertRow("POTable", shim.Row{
		Columns: []*shim.Column{
			&shim.Column{Value: &shim.Column_String_{String_: "DOC"}},
			&shim.Column{Value: &shim.Column_String_{String_: UID}},
			&shim.Column{Value: &shim.Column_Bytes{Bytes: docJSON}},
			//&shim.Column{Value: &shim.Column_Bytes{Bytes: docPDF}},
			&shim.Column{Value: &shim.Column_String_{String_: "SUBMITTED_BY_IB"}}},
	})

	if !ok && err == nil {
		return nil, errors.New("Document already exists.")
	}

	return nil, err
}


//Update PO

func (t *PO) UpdatePO(stub shim.ChaincodeStubInterface, args []string) ([]byte, error) {

	if len(args) != 2{
		return nil, errors.New("Incorrect number of arguments. Expecting 2.")
	}

	UID := args[0]
	docJSON := []byte(args[1])
	
	var columns []shim.Column
	col1 := shim.Column{Value: &shim.Column_String_{String_: "DOC"}}
	columns = append(columns, col1)
	col2 := shim.Column{Value: &shim.Column_String_{String_: UID}}
	columns = append(columns, col2)
	

	row, err := stub.GetRow("POTable", columns)
	if err != nil {
		fmt.Errorf("Error: Failed retrieving document with UID %s. Error %s", UID, err.Error())
	}

	// GetRows returns empty message if key does not exist
	if len(row.Columns) == 0 {
		fmt.Errorf("Eror in getting column")
	}


	ok, err := stub.ReplaceRow("POTable", shim.Row{
		Columns: []*shim.Column{
			&shim.Column{Value: &shim.Column_String_{String_: "DOC"}},
			&shim.Column{Value: &shim.Column_String_{String_: UID}},
			&shim.Column{Value: &shim.Column_Bytes{Bytes: docJSON}},
			&shim.Column{Value: &shim.Column_String_{String_: "PAYMENT_INITIATED"}},
			
	}})

	if !ok && err == nil {

		return nil, errors.New("Document unable to Update.")
	}


	return nil, err

	
}
/*
//UpdateStatus () – Updates current document Status. Enforces Status transition logic.
func (t *PO) UpdateStatus(stub shim.ChaincodeStubInterface, args []string) ([]byte, error) {

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

	row, err := stub.GetRow("POTable", columns)
	if err != nil {
		return nil, fmt.Errorf("Error: Failed retrieving document with UID %s. Error %s", UID, err.Error())
	}

	// GetRows returns empty message if key does not exist
	if len(row.Columns) == 0 {
		return nil, nil
	}

	docJSON := row.Columns[2].GetBytes()
	//docPDF := row.Columns[3].GetBytes()
	currStatus := row.Columns[3].GetString_()

	//Start- Check that the currentStatus to newStatus transition is accurate

	stateTransitionAllowed := false

	if currStatus == "SUBMITTED_BY_IB" && newStatus == "ACCEPTED_BY_EB" {
		stateTransitionAllowed = true
	} else if currStatus == "SUBMITTED_BY_IB" && newStatus == "REJECTED_BY_EB" {
		stateTransitionAllowed = true
	} else if currStatus == "SUBMITTED_BY_IB" && newStatus == "PAYMENT_DUE_FROM_IB_TO_EB" {
		stateTransitionAllowed = true
	} else if currStatus == "ACCEPTED_BY_EB" && newStatus == "PAYMENT_DUE_FROM_IB_TO_EB" {
		stateTransitionAllowed = true
	} else if currStatus == "PAYMENT_DUE_FROM_IB_TO_EB" && newStatus == "PAYMENT_RECEIVED" {
		stateTransitionAllowed = true
	} else if currStatus == "PAYMENT_DUE_FROM_IB_TO_EB" && newStatus == "PAYMENT_DEFAULTED" {
		stateTransitionAllowed = true
	}

	if stateTransitionAllowed == false {
		return nil, errors.New("This state transition is not allowed.")
	}

	//End- Check that the currentStatus to newStatus transition is accurate

	err = stub.DeleteRow(
		"POTable",
		columns,
	)
	if err != nil {
		return nil, errors.New("Failed deleting row.")
	}

	_, err = stub.InsertRow(
		"POTable",
		shim.Row{
			Columns: []*shim.Column{
				&shim.Column{Value: &shim.Column_String_{String_: "DOC"}},
				&shim.Column{Value: &shim.Column_String_{String_: UID}},
				&shim.Column{Value: &shim.Column_Bytes{Bytes: docJSON}},
				//&shim.Column{Value: &shim.Column_Bytes{Bytes: docPDF}},
				&shim.Column{Value: &shim.Column_String_{String_: newStatus}}},
		})
	if err != nil {
		return nil, errors.New("Failed inserting row.")
	}

	return nil, nil

}
*/

// GetJSON () – returns as JSON a single document w.r.t. the UID
func (t *PO) GetJSON(stub shim.ChaincodeStubInterface, args []string) ([]byte, error) {

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

	row, err := stub.GetRow("POTable", columns)
	if err != nil {
		return nil, fmt.Errorf("Error: Failed retrieving document with UID %s. Error %s", UID, err.Error())
	}

	// GetRows returns empty message if key does not exist
	if len(row.Columns) == 0 {
		return nil, nil
	}

	return row.Columns[2].GetBytes(), nil

}

/*
// GetPDF () – returns as JSON a single document w.r.t. the UID
func (t *PO) GetPDF(stub shim.ChaincodeStubInterface, args []string) ([]byte, error) {

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

	row, err := stub.GetRow("LCTable", columns)
	if err != nil {
		jsonResp := "{\"Error\":\"Failed retrieveing document with UID " + UID + ". Error " + err.Error() + ". \"}"
		return nil, errors.New(jsonResp)
	}

	// GetRows returns empty message if key does not exist
	if len(row.Columns) == 0 {
		return nil, nil
	}

	return row.Columns[3].GetBytes(), nil
}
*/

/*
// GetStatus () – returns as JSON the Status w.r.t. the UID
func (t *PO) GetStatus(stub shim.ChaincodeStubInterface, args []string) ([]byte, error) {

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

	row, err := stub.GetRow("POTable", columns)
	if err != nil {
		return nil, fmt.Errorf("Error: Failed retrieving document with UID %s. Error %s", UID, err.Error())
	}

	// GetRows returns empty message if key does not exist
	if len(row.Columns) == 0 {
		return nil, nil
	}

	return []byte(row.Columns[3].GetString_()), nil
}
*/