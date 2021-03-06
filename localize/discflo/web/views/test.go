package views

import (
	"fmt"
	"strconv"
)

func inSlice(length int) func(idx int) bool {
	return func(idx int) bool {
		return idx >= 0 && idx < length
	}
}

func (c *Context) indexIn(name string, has func(int) bool) (int, error) {
	sid := c.p.ByName(name)
	id, err := strconv.Atoi(sid)
	if err != nil {
		return 0, fmt.Errorf("Expected an int got `%v` for :%v part. err: %v", sid, name, err)
	}
	if !has(id) {
		return 0, fmt.Errorf("%v was less out of range.", name)
	}
	return id, nil
}

func (v *Views) GenerateTest(c *Context) error {
	type data struct {
		ClusterId int
		NodeId    int
		TestId    int
		From      string
		Test      string
		Stdout    string
		Stderr    string
		Original  string
	}
	clusters, err := v.localization.Clusters()
	if err != nil {
		return err
	}
	tid, err := c.indexIn("tid", clusters.HasTest)
	if err != nil {
		return err
	}
	cid, err := c.indexIn("cid", clusters.Has)
	if err != nil {
		return err
	}
	cluster := clusters.Get(cid)
	if cluster == nil {
		return fmt.Errorf("cluster %v was nil", cid)
	}
	nid, err := c.indexIn("nid", inSlice(len(cluster.Nodes)))
	if err != nil {
		return err
	}
	tc, err := clusters.Test(tid, cid, nid)
	if err != nil {
		return err
	}
	var test, out, errout string
	if tc != nil {
		test = string(tc.Case)
		stdout, stderr, _, _, _, err := tc.Exec.Execute(tc.Case)
		if err != nil {
			return err
		}
		out = string(stdout)
		errout = string(stderr)
	}
	from, original := v.localization.Test(tid)
	return v.tmpl.ExecuteTemplate(c.rw, "test", &data{
		ClusterId: cid,
		NodeId:    nid,
		TestId:    tid,
		From:      from,
		Test:      test,
		Stdout:    out,
		Stderr:    errout,
		Original:  string(original),
	})
}
