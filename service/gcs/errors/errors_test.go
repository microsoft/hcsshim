package errors

import (
	"fmt"

	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
	"github.com/pkg/errors"
)

type stackTraceError interface {
	error
	StackTrace() errors.StackTrace
}
type causeError interface {
	error
	Cause() error
}
type stackTraceCauseError interface {
	stackTraceError
	Cause() error
}

var _ = Describe("Errors", func() {
	Describe("unittests", func() {
		Describe("getting the base stack trace", func() {
			Context("only one error in cause stack", func() {
				var (
					e          error
					stackTrace errors.StackTrace
				)
				BeforeEach(func() {
					e = errors.New("test")
					stackTrace = BaseStackTrace(e)
				})
				It("should return the same stack trace as the single error", func() {
					se, ok := e.(StackTracer)
					Expect(ok).To(BeTrue())
					Expect(stackTrace).To(Equal(se.StackTrace()))
				})
			})
			Context("multiple StackTracers in cause stack", func() {
				var (
					e1         error
					e2         error
					e3         error
					e4         error
					stackTrace errors.StackTrace
				)
				BeforeEach(func() {
					e1 = fmt.Errorf("test")
					e2 = errors.WithStack(e1)
					e3 = errors.WithStack(e2)
					e4 = errors.WithMessage(e3, "test2")
					stackTrace = BaseStackTrace(e4)
				})
				It("should return the same stack trace as the bottom-most StackTracer", func() {
					se, ok := e2.(StackTracer)
					Expect(ok).To(BeTrue())
					Expect(stackTrace).To(Equal(se.StackTrace()))
				})
			})
			Context("no StackTracers in cause stack", func() {
				var (
					e          error
					stackTrace errors.StackTrace
				)
				BeforeEach(func() {
					e = fmt.Errorf("test")
					stackTrace = BaseStackTrace(e)
				})
				It("should return nil", func() {
					Expect(stackTrace).To(BeNil())
				})
			})
		})
		Describe("constructing HRESULT errors", func() {
			Context("with no wrapped error", func() {
				var (
					herr *baseHresultError
				)
				BeforeEach(func() {
					e := NewHresultError(HrInvalidArg)
					var ok bool
					herr, ok = e.(*baseHresultError)
					Expect(ok).To(BeTrue())
				})
				It("should have the correct HRESULT value", func() {
					Expect(herr.Hresult()).To(Equal(HrInvalidArg))
				})
				It("should have the correct error string", func() {
					Expect(herr.Error()).To(Equal("HRESULT: 0x80070057"))
				})
			})
			Context("with a wrapped errorString", func() {
				var (
					herr       *wrappingHresultError
					wrappedErr error
				)
				BeforeEach(func() {
					wrappedErr = fmt.Errorf("wrapped %s", "error")
					e := WrapHresult(wrappedErr, HrInvalidArg)
					var ok bool
					herr, ok = e.(*wrappingHresultError)
					Expect(ok).To(BeTrue())
				})
				It("should have the correct HRESULT value", func() {
					Expect(herr.Hresult()).To(Equal(HrInvalidArg))
				})
				It("should have a cause equal to wrappedErr", func() {
					Expect(herr.Cause()).To(Equal(wrappedErr))
				})
				It("should have an error string equal to that of wrappedErr", func() {
					Expect(herr.Error()).To(Equal("HRESULT 0x80070057: " + wrappedErr.Error()))
				})
				It("should have a nil stack trace", func() {
					Expect(herr.StackTrace()).To(BeNil())
				})
			})
			Context("with a wrapped pkg/errors error", func() {
				var (
					herr       *wrappingHresultError
					wrappedErr stackTraceError
				)
				BeforeEach(func() {
					e1 := errors.New("wrapped error")
					var ok bool
					wrappedErr, ok = e1.(stackTraceError)
					Expect(ok).To(BeTrue())
					e2 := WrapHresult(wrappedErr, HrInvalidArg)
					herr, ok = e2.(*wrappingHresultError)
					Expect(ok).To(BeTrue())
				})
				It("should have the correct HRESULT value", func() {
					Expect(herr.Hresult()).To(Equal(HrInvalidArg))
				})
				It("should have a cause equal to wrappedErr", func() {
					Expect(herr.Cause()).To(Equal(wrappedErr))
				})
				It("should have a stack trace equal to that of wrappedErr", func() {
					Expect(herr.StackTrace()).To(Equal(wrappedErr.StackTrace()))
				})
				It("should have an error string equal to that of wrappedErr", func() {
					Expect(herr.Error()).To(Equal("HRESULT 0x80070057: " + wrappedErr.Error()))
				})
			})
		})
		Describe("creating cause stacks", func() {
			Context("baseHresultError is direct cause", func() {
				var (
					herr   *baseHresultError
					pkgErr causeError
				)
				BeforeEach(func() {
					e1 := NewHresultError(HrInvalidArg)
					var ok bool
					herr, ok = e1.(*baseHresultError)
					Expect(ok).To(BeTrue())
					e2 := errors.Wrap(herr, "stackErr")
					pkgErr, ok = e2.(causeError)
					Expect(ok).To(BeTrue())
				})
				It("should have the correct HRESULT value", func() {
					Expect(GetHresult(pkgErr)).To(Equal(HrInvalidArg))
				})
				It("should have herr as its cause", func() {
					Expect(errors.Cause(pkgErr)).To(Equal(herr))
				})
			})
			Context("wrappingHresultError is direct cause", func() {
				var (
					wrappedErr error
					herr       *wrappingHresultError
					pkgErr     causeError
				)
				BeforeEach(func() {
					wrappedErr = errors.New("wrapped error")
					e1 := WrapHresult(wrappedErr, HrInvalidArg)
					var ok bool
					herr, ok = e1.(*wrappingHresultError)
					Expect(ok).To(BeTrue())
					e2 := errors.Wrap(herr, "pkgErr")
					pkgErr, ok = e2.(causeError)
					Expect(ok).To(BeTrue())
				})
				It("should have the correct HRESULT value", func() {
					Expect(GetHresult(pkgErr)).To(Equal(HrInvalidArg))
				})
				It("should have wrappedErr as its cause", func() {
					Expect(errors.Cause(pkgErr)).To(Equal(wrappedErr))
				})
			})
			Context("multiple HRESULT errors in the cause stack", func() {
				var (
					wrappedErr error
					herr1      *wrappingHresultError
					herr2      *wrappingHresultError
					pkgErr     stackTraceCauseError
				)
				BeforeEach(func() {
					wrappedErr = errors.New("wrapped error")
					e1 := WrapHresult(wrappedErr, HrInvalidArg)
					var ok bool
					herr1, ok = e1.(*wrappingHresultError)
					Expect(ok).To(BeTrue())
					e2 := errors.Wrap(herr1, "pkgErr")
					pkgErr, ok = e2.(stackTraceCauseError)
					Expect(ok).To(BeTrue())
					e3 := WrapHresult(pkgErr, HrNotImpl)
					herr2, ok = e3.(*wrappingHresultError)
					Expect(ok).To(BeTrue())
				})
				It("should have the correct HRESULT value", func() {
					Expect(GetHresult(herr2)).To(Equal(HrNotImpl))
				})
				It("should have wrappedErr as its cause", func() {
					Expect(errors.Cause(pkgErr)).To(Equal(wrappedErr))
				})
				It("should have the same stack as pkgErr", func() {
					Expect(herr2.StackTrace()).To(Equal(pkgErr.StackTrace()))
				})
			})
		})
	})
})
