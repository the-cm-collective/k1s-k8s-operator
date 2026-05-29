package controller

import (
	"context"
	"fmt"
	"net"
	"net/http"
	"strconv"
	"strings"
	"time"

	operatorv1alpha1 "github.com/k1s-project/k1s-operator/api/v1alpha1"
)

type TrafficProbeFunc func(context.Context, *operatorv1alpha1.K1sExposure) error

func defaultTrafficProbe(ctx context.Context, exposure *operatorv1alpha1.K1sExposure) error {
	spec := exposure.Spec.TrafficProbe
	timeout := time.Duration(defaultI32(spec.TimeoutSeconds, 3)) * time.Second
	probeCtx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()

	port := defaultI32(spec.Port, exposureServicePort(exposure))
	host := exposureServiceName(exposure) + "." + exposure.Namespace + ".svc"
	address := net.JoinHostPort(host, strconv.Itoa(int(port)))
	probeType := spec.Type
	if probeType == "" {
		probeType = operatorv1alpha1.K1sTrafficProbeTypeHTTP
	}
	if probeType == operatorv1alpha1.K1sTrafficProbeTypeTCP {
		var dialer net.Dialer
		conn, err := dialer.DialContext(probeCtx, "tcp", address)
		if err != nil {
			return err
		}
		return conn.Close()
	}

	scheme := spec.Scheme
	if scheme == "" {
		scheme = operatorv1alpha1.K1sTrafficProbeSchemeHTTP
	}
	path := spec.Path
	if path == "" {
		path = exposurePath(exposure)
	}
	req, err := http.NewRequestWithContext(probeCtx, http.MethodGet, strings.ToLower(string(scheme))+"://"+address+path, nil)
	if err != nil {
		return err
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	minStatus, maxStatus, err := expectedStatusRange(spec.ExpectedStatus)
	if err != nil {
		return err
	}
	if resp.StatusCode < minStatus || resp.StatusCode > maxStatus {
		return fmt.Errorf("traffic probe returned HTTP %d", resp.StatusCode)
	}
	return nil
}

func expectedStatusRange(raw string) (int, int, error) {
	if raw == "" {
		return 200, 399, nil
	}
	parts := strings.SplitN(raw, "-", 2)
	minStatus, err := strconv.Atoi(parts[0])
	if err != nil {
		return 0, 0, err
	}
	maxStatus := minStatus
	if len(parts) == 2 {
		maxStatus, err = strconv.Atoi(parts[1])
		if err != nil {
			return 0, 0, err
		}
	}
	if minStatus > maxStatus {
		return 0, 0, fmt.Errorf("expectedStatus lower bound %d is greater than upper bound %d", minStatus, maxStatus)
	}
	return minStatus, maxStatus, nil
}
