package server

import (
	"encoding/xml"
	"github.com/hneemann/quiz/data"
	"github.com/stretchr/testify/assert"
	"net/http/httptest"
	"strings"
	"testing"
)

func Test_getStrFromPath(t *testing.T) {
	tests := []struct {
		path string
		v    string
		n    string
	}{
		{"/task/2/3", "3", "/task/2"},
		{"/task/2/3/", "3", "/task/2"},
	}
	for _, tt := range tests {
		t.Run(tt.path, func(t *testing.T) {
			v, n := getStrFromPath(tt.path)
			if v != tt.v {
				t.Errorf("getStrFromPath() got = %v, want %v", v, tt.v)
			}
			if n != tt.n {
				t.Errorf("getStrFromPath() got1 = %v, want %v", n, tt.n)
			}
		})
	}
}

func readLectureToTest(xmlStr string) (*data.Lecture, error) {
	var l data.Lecture
	err := xml.Unmarshal([]byte(xmlStr), &l)
	if err != nil {
		return nil, err
	}
	err = l.Init()
	if err != nil {
		return nil, err
	}
	return &l, nil
}

func Test_MathMLInResult(t *testing.T) {
	const lecture = `<Lecture id="ET1">
    <Title>Elektrotechnik 1</Title>
    <Author>Prof. Dr. Helmut Neemann</Author>
    <AuthorEMail>helmut.neemann@dhbw.de</AuthorEMail>
    <Chapter>
        <Title>Func</Title>
        <Task>
            <Input id="val1" type="text">
                <Label>Func:</Label>
                <Validator>
                    <Expression>"Found "+parseFunc(answer.val1,["x"]).mathMl()+" in Result"</Expression>
                </Validator>
            </Input>
        </Task>
	</Chapter>
</Lecture>`

	lec, err := readLectureToTest(lecture)
	assert.NoError(t, err)

	states := &data.LectureStates{}
	lectures := &data.Lectures{}
	lectures.Insert(lec)
	h := CreateTask(lectures, states)

	r := httptest.NewRequest("POST", "/task/ET1/0/0", nil)
	r.Form = map[string][]string{"input_val1": {"2/x"}}
	w := httptest.NewRecorder()
	h.ServeHTTP(w, r)

	assert.Equal(t, 200, w.Code)

	//Ensure that the MathML is in the result
	assert.True(t, strings.Contains(w.Body.String(), "><mfrac><mn>2</mn><mi>x</mi></mfrac></math> in Result"))
}
