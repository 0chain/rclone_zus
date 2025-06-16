// This file contains the implementation of the Batcher interface

package zus

import (
	"fmt"
	"context"
	"github.com/0chain/gosdk/zboxcore/sdk"
)

func (f *Fs) commitBatch(ctx context.Context, operations []sdk.OperationRequest, results []struct{}, errors []error) error {
	//Add log
	fmt.Printf("Committing batch with %d operations\n", len(operations))

	if err := f.alloc.DoMultiOperation(operations); err != nil {
		return err
	}
	return nil
}
