// Package zkio is the guest's interface to the zkVM: ReadInput returns the
// input record, Commit writes the public values, and Succeed or Fail halt the
// machine and never return. Each zkVM provides them under its build tag.
package zkio
