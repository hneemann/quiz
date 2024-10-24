package data

import (
	"bytes"
	"github.com/stretchr/testify/assert"
	"strings"
	"testing"
)

func TestInit(t *testing.T) {
	tests := []struct {
		expectedError string
		xml           string
	}{
		{
			expectedError: "'val2' is used but not available",
			xml: `<Lecture id="ET1">
    <Title>Elektrotechnik 1</Title>
    <Author>Prof. Dr. Helmut Neemann</Author>
    <AuthorEMail>helmut.neemann@dhbw.de</AuthorEMail>
    <Chapter>
        <Title>Gleichstromkreise</Title>
        <Task>
            <Question></Question>
            <Input id="val1" type="text">
                <Label>$U_Q/\u{V}$:</Label>
                <Validator>
                    <Expression>cmpValues(40,a.val2,1)</Expression>
                    <Explanation>Der Wert beträgt $U_Q=40\u{V}$.</Explanation>
                </Validator>
            </Input>
        </Task>
	</Chapter>
</Lecture>`},
		{
			expectedError: "validator is missing",
			xml: `<Lecture id="ET1">
    <Title>Elektrotechnik 1</Title>
    <Author>Prof. Dr. Helmut Neemann</Author>
    <AuthorEMail>helmut.neemann@dhbw.de</AuthorEMail>
    <Chapter>
        <Title>Gleichstromkreise</Title>
        <Task>
            <Question></Question>
            <Input id="val1" type="text">
                <Label>$U_Q/\u{V}$:</Label>
            </Input>
        </Task>
	</Chapter>
</Lecture>`},
		{
			expectedError: "'val2' is used but not available",
			xml: `<Lecture id="ET1">
    <Title>Elektrotechnik 1</Title>
    <Author>Prof. Dr. Helmut Neemann</Author>
    <AuthorEMail>helmut.neemann@dhbw.de</AuthorEMail>
    <Chapter>
        <Title>Gleichstromkreise</Title>
        <Task>
            <Question></Question>
            <Input id="val1" type="text">
                <Label>$U_Q/\u{V}$:</Label>
            </Input>
            <Validator>
                <Expression>cmpValues(40,a.val2,1)</Expression>
                <Explanation>Der Wert beträgt $U_Q=40\u{V}$.</Explanation>
            </Validator>
        </Task>
	</Chapter>
</Lecture>`},
		{
			expectedError: "'val1' is not used in expression",
			xml: `<Lecture id="ET1">
    <Title>Elektrotechnik 1</Title>
    <Author>Prof. Dr. Helmut Neemann</Author>
    <AuthorEMail>helmut.neemann@dhbw.de</AuthorEMail>
    <Chapter>
        <Title>Gleichstromkreise</Title>
        <Task>
            <Question></Question>
            <Input id="val1" type="text">
                <Label>$U_Q/\u{V}$:</Label>
            </Input>
            <Input id="val2" type="text">
                <Label>$U_Q/\u{V}$:</Label>
            </Input>
            <Validator>
                <Expression>cmpValues(40,a.val2,1)</Expression>
                <Explanation>Der Wert beträgt $U_Q=40\u{V}$.</Explanation>
            </Validator>
        </Task>
	</Chapter>
</Lecture>`},
		{
			expectedError: "duplicate input",
			xml: `<Lecture id="ET1">
    <Title>Elektrotechnik 1</Title>
    <Author>Prof. Dr. Helmut Neemann</Author>
    <AuthorEMail>helmut.neemann@dhbw.de</AuthorEMail>
    <Chapter>
        <Title>Gleichstromkreise</Title>
        <Task>
            <Question></Question>
            <Input id="val1" type="text">
                <Label>$U_Q/\u{V}$:</Label>
            </Input>
            <Input id="val1" type="text">
                <Label>$U_Q/\u{V}$:</Label>
            </Input>
            <Validator>
                <Expression>cmpValues(40,a.val2,1)</Expression>
                <Explanation>Der Wert beträgt $U_Q=40\u{V}$.</Explanation>
            </Validator>
        </Task>
	</Chapter>
</Lecture>`},
		{
			expectedError: "no label at input",
			xml: `<Lecture id="ET1">
    <Title>Elektrotechnik 1</Title>
    <Author>Prof. Dr. Helmut Neemann</Author>
    <AuthorEMail>helmut.neemann@dhbw.de</AuthorEMail>
    <Chapter>
        <Title>Gleichstromkreise</Title>
        <Task>
            <Question></Question>
            <Input id="val1" type="text">
                <Label></Label>
                <Validator>
                    <Expression>cmpValues(40,a.val1,1)</Expression>
                    <Explanation>Der Wert beträgt $U_Q=40\u{V}$.</Explanation>
                </Validator>
            </Input>
        </Task>
	</Chapter>
</Lecture>`},
	}

	for _, tt := range tests {
		t.Run(tt.expectedError, func(t *testing.T) {
			b := bytes.NewBufferString(tt.xml)
			_, err := New(b)
			if tt.expectedError == "" {
				assert.NoError(t, err)
			} else {
				ok := strings.Contains(err.Error(), tt.expectedError)
				if !ok {
					t.Log(err)
				}
				assert.True(t, ok)
			}
		})
	}
}

func TestTest(t *testing.T) {
	tests := []struct {
		expectedError string
		xml           string
	}{
		{
			expectedError: "",
			xml: `<Lecture id="ET1">
    <Title>Elektrotechnik 1</Title>
    <Author>Prof. Dr. Helmut Neemann</Author>
    <AuthorEMail>helmut.neemann@dhbw.de</AuthorEMail>
    <Chapter>
        <Title>Gleichstromkreise</Title>
        <Task>
            <Question></Question>
            <Input id="val1" type="text">
                <Label>$U_Q/\u{V}$:</Label>
                <Validator>
                    <Expression>cmpValues(40,a.val1,1)</Expression>
                    <Test val1="40" ok="yes"/>
                    <Test val1="41" ok="no"/>
                    <Test val1="39" ok="no"/>
                    <Explanation>Der Wert beträgt $U_Q=40\u{V}$.</Explanation>
                </Validator>
            </Input>
        </Task>
	</Chapter>
</Lecture>`},
		{
			expectedError: "unknown variable 'val2'",
			xml: `<Lecture id="ET1">
    <Title>Elektrotechnik 1</Title>
    <Author>Prof. Dr. Helmut Neemann</Author>
    <AuthorEMail>helmut.neemann@dhbw.de</AuthorEMail>
    <Chapter>
        <Title>Gleichstromkreise</Title>
        <Task>
            <Question></Question>
            <Input id="val1" type="text">
                <Label>$U_Q/\u{V}$:</Label>
                <Validator>
                    <Expression>cmpValues(40,a.val1,1)</Expression>
                    <Test val2="40" ok="yes"/>
                    <Explanation>Der Wert beträgt $U_Q=40\u{V}$.</Explanation>
                </Validator>
            </Input>
        </Task>
	</Chapter>
</Lecture>`},
		{
			expectedError: "not '40'",
			xml: `<Lecture id="ET1">
    <Title>Elektrotechnik 1</Title>
    <Author>Prof. Dr. Helmut Neemann</Author>
    <AuthorEMail>helmut.neemann@dhbw.de</AuthorEMail>
    <Chapter>
        <Title>Gleichstromkreise</Title>
        <Task>
            <Question></Question>
            <Input id="val1" type="checkbox">
                <Label>$U_Q/\u{V}$:</Label>
                <Validator>
                    <Expression>a.val1</Expression>
                    <Test val1="40" ok="yes"/>
                    <Explanation>Der Wert beträgt $U_Q=40\u{V}$.</Explanation>
                </Validator>
            </Input>
        </Task>
	</Chapter>
</Lecture>`},
		{
			expectedError: "",
			xml: `<Lecture id="ET1">
    <Title>Elektrotechnik 1</Title>
    <Author>Prof. Dr. Helmut Neemann</Author>
    <AuthorEMail>helmut.neemann@dhbw.de</AuthorEMail>
    <Chapter>
        <Title>Gleichstromkreise</Title>
        <Task>
            <Question></Question>
            <Input id="val1" type="checkbox">
                <Label>$U_Q/\u{V}$:</Label>
                <Validator>
                    <Expression>a.val1</Expression>
                    <Test val1="true" ok="yes"/>
                    <Explanation>Der Wert beträgt $U_Q=40\u{V}$.</Explanation>
                </Validator>
            </Input>
        </Task>
	</Chapter>
</Lecture>`},
		{
			expectedError: "expected 'Teststring2', got '\"Teststring\"'",
			xml: `<Lecture id="ET1">
    <Title>Elektrotechnik 1</Title>
    <Author>Prof. Dr. Helmut Neemann</Author>
    <AuthorEMail>helmut.neemann@dhbw.de</AuthorEMail>
    <Chapter>
        <Title>Gleichstromkreise</Title>
        <Task>
            <Question></Question>
            <Input id="val1" type="checkbox">
                <Label>$U_Q/\u{V}$:</Label>
                <Validator>
                    <Expression>if a.val1 then "Teststring" else true</Expression>
                    <Test val1="true">Teststring2</Test>
                    <Explanation>Der Wert beträgt $U_Q=40\u{V}$.</Explanation>
                </Validator>
            </Input>
        </Task>
	</Chapter>
</Lecture>`},
		{
			expectedError: "",
			xml: `<Lecture id="ET1">
    <Title>Elektrotechnik 1</Title>
    <Author>Prof. Dr. Helmut Neemann</Author>
    <AuthorEMail>helmut.neemann@dhbw.de</AuthorEMail>
    <Chapter>
        <Title>Gleichstromkreise</Title>
        <Task>
            <Question></Question>
            <Input id="val1" type="checkbox">
                <Label>$U_Q/\u{V}$:</Label>
                <Validator>
                    <Expression>if a.val1 then "Teststring" else true</Expression>
                    <Test val1="true">Teststring</Test>
                    <Test val1="false" ok="yes"/>
                    <Explanation>Der Wert beträgt $U_Q=40\u{V}$.</Explanation>
                </Validator>
            </Input>
        </Task>
	</Chapter>
</Lecture>`},
	}

	for _, tt := range tests {
		t.Run(tt.expectedError, func(t *testing.T) {
			_, err := New(strings.NewReader(tt.xml))
			if tt.expectedError == "" {
				assert.NoError(t, err)
			} else {
				if err == nil {
					t.Errorf("expected an error containing '%s'", tt.expectedError)
					return
				}
				ok := strings.Contains(err.Error(), tt.expectedError)
				if !ok {
					t.Log(err)
				}
				assert.True(t, ok)
			}
		})
	}
}
