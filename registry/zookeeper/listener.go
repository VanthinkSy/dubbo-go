/*
 * Licensed to the Apache Software Foundation (ASF) under one or more
 * contributor license agreements.  See the NOTICE file distributed with
 * this work for additional information regarding copyright ownership.
 * The ASF licenses this file to You under the Apache License, Version 2.0
 * (the "License"); you may not use this file except in compliance with
 * the License.  You may obtain a copy of the License at
 *
 *     http://www.apache.org/licenses/LICENSE-2.0
 *
 * Unless required by applicable law or agreed to in writing, software
 * distributed under the License is distributed on an "AS IS" BASIS,
 * WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
 * See the License for the specific language governing permissions and
 * limitations under the License.
 */

package zookeeper

import (
	"context"
	"strings"
)
import (
	perrors "github.com/pkg/errors"
)
import (
	"github.com/apache/dubbo-go/common"
	"github.com/apache/dubbo-go/common/logger"
	"github.com/apache/dubbo-go/registry"
	"github.com/apache/dubbo-go/remoting"
	zk "github.com/apache/dubbo-go/remoting/zookeeper"
)

type RegistryDataListener struct {
	interestedURL []*common.URL
	listener      remoting.ConfigurationListener
}

func NewRegistryDataListener(listener remoting.ConfigurationListener) *RegistryDataListener {
	return &RegistryDataListener{listener: listener, interestedURL: []*common.URL{}}
}
func (l *RegistryDataListener) AddInterestedURL(url *common.URL) {
	l.interestedURL = append(l.interestedURL, url)
}

func (l *RegistryDataListener) DataChange(eventType remoting.Event) bool {
	// Intercept the last bit
	url := eventType.Path[strings.Index(eventType.Path, "/providers/")+len("/providers/"):]
	serviceURL, err := common.NewURL(context.TODO(), url)
	if err != nil {
		logger.Errorf("Listen NewURL(r{%s}) = error{%v}", url, err)
		return false
	}
	for _, v := range l.interestedURL {
		if serviceURL.URLEqual(*v) {
			l.listener.Process(&remoting.ConfigChangeEvent{Value: serviceURL, ConfigType: eventType.Action})
			return true
		}
	}

	return false
}

type RegistryConfigurationListener struct {
	client   *zk.ZookeeperClient
	registry *zkRegistry
	events   chan *remoting.ConfigChangeEvent
}

func NewRegistryConfigurationListener(client *zk.ZookeeperClient, reg *zkRegistry) *RegistryConfigurationListener {
	reg.wg.Add(1)
	return &RegistryConfigurationListener{client: client, registry: reg, events: make(chan *remoting.ConfigChangeEvent, 32)}
}
func (l *RegistryConfigurationListener) Process(configType *remoting.ConfigChangeEvent) {
	l.events <- configType
}

func (l *RegistryConfigurationListener) Next() (*registry.ServiceEvent, error) {
	for {
		select {
		case <-l.client.Done():
			logger.Warnf("listener's zk client connection is broken, so zk event listener exit now.")
			return nil, perrors.New("listener stopped")

		case <-l.registry.done:
			logger.Warnf("zk consumer register has quit, so zk event listener exit asap now.")
			return nil, perrors.New("listener stopped")

		case e := <-l.events:
			logger.Debugf("got zk event %s", e)
			if e.ConfigType == remoting.EventTypeDel && !l.valid() {
				logger.Warnf("update @result{%s}. But its connection to registry is invalid", e.Value)
				continue
			}
			//r.update(e.res)
			//write to invoker
			//r.outerEventCh <- e.res
			return &registry.ServiceEvent{Action: e.ConfigType, Service: e.Value.(common.URL)}, nil
		}
	}
}
func (l *RegistryConfigurationListener) Close() {
	l.registry.wg.Done()
}

func (l *RegistryConfigurationListener) valid() bool {
	return l.client.ZkConnValid()
}
