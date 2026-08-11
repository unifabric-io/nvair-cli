package create

import "errors"

var errOOBMgmtServerNotFound = errors.New("oob-mgmt-server node not found in simulation")

func joinErrors(errCh <-chan error) error {
	var errs []error
	for err := range errCh {
		if err != nil {
			errs = append(errs, err)
		}
	}

	return errors.Join(errs...)
}
