package repository

// Runtime-variable materialization decrypts function secrets for the trusted
// execution worker. It is a cipher/snapshot concern rather than execution
// lifecycle, so it lives in its own file.

import (
	"context"
	"strings"

	"github.com/Nafixhutao/stealth-forge/internal/functionsecret"
	"github.com/google/uuid"
)

// FunctionRuntimeVariables decrypts values only for the trusted execution
// worker. It never returns metadata to an API actor and rejects malformed
// ciphertext rather than starting a process with partial configuration.
func (r *Repository) FunctionRuntimeVariables(
	ctx context.Context,
	projectID, functionID uuid.UUID,
	cipher *functionsecret.Cipher,
) ([]FunctionRuntimeVariable, error) {
	if cipher == nil {
		return nil, ErrFunctionSecretUnavailable
	}
	if _, err := r.functionByID(ctx, r.pool, projectID, functionID, false); err != nil {
		return nil, err
	}
	rows, err := r.pool.Query(
		ctx,
		`SELECT key,value_ciphertext,is_secret FROM function_variables WHERE project_id=$1 AND function_id=$2 ORDER BY key`,
		projectID,
		functionID,
	)
	if err != nil {
		return nil, err
	}
	return materializeFunctionRuntimeVariables(rows, cipher)
}

// FunctionRuntimeVariablesForDeployment decrypts the ciphertext snapshot that
// belongs to one immutable deployment. Updating function_variables therefore
// cannot silently alter a built revision or an invocation already queued for
// it.
func (r *Repository) FunctionRuntimeVariablesForDeployment(
	ctx context.Context,
	projectID, functionID, deploymentID uuid.UUID,
	cipher *functionsecret.Cipher,
) ([]FunctionRuntimeVariable, error) {
	if cipher == nil {
		return nil, ErrFunctionSecretUnavailable
	}
	var exists bool
	if err := r.pool.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM function_deployments WHERE project_id=$1 AND function_id=$2 AND id=$3)`, projectID, functionID, deploymentID).Scan(&exists); err != nil {
		return nil, err
	}
	if !exists {
		return nil, ErrNotFound
	}
	rows, err := r.pool.Query(
		ctx,
		`SELECT key,value_ciphertext,is_secret FROM function_deployment_variables WHERE project_id=$1 AND function_id=$2 AND deployment_id=$3 ORDER BY key`,
		projectID,
		functionID,
		deploymentID,
	)
	if err != nil {
		return nil, err
	}
	return materializeFunctionRuntimeVariables(rows, cipher)
}

func materializeFunctionRuntimeVariables(
	rows functionVariableRows,
	cipher *functionsecret.Cipher,
) ([]FunctionRuntimeVariable, error) {
	defer rows.Close()
	variables := make([]FunctionRuntimeVariable, 0)
	for rows.Next() {
		var key string
		var ciphertext []byte
		var secret bool
		if err := rows.Scan(&key, &ciphertext, &secret); err != nil {
			return nil, err
		}
		if len(ciphertext) == 0 {
			return nil, ErrFunctionSecretUnavailable
		}
		plaintext, err := cipher.Decrypt(ciphertext)
		if err != nil || strings.IndexByte(string(plaintext), 0) >= 0 {
			return nil, ErrFunctionSecretUnavailable
		}
		variables = append(variables, FunctionRuntimeVariable{Key: key, Value: string(plaintext), IsSecret: secret})
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	return variables, nil
}
