package main

import (
	"k8s.io/apimachinery/pkg/labels"
	"time"

	log "github.com/sirupsen/logrus"

	"k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/tools/cache"
)

// Remove all obsolete objects from Icinga.
func (c *Controller) IcingaHousekeeping() {
	for {
		c.IcingaHostGroupHousekeeping()
		c.IcingaHostHousekeeping()
		c.IcingaCheckHousekeeping()
		c.CrHousekeeping()
		time.Sleep(60 * time.Second)
	}
}

func (c *Controller) IcingaHostGroupHousekeeping() {
	hostgroups, err := c.Icinga.ListHostGroups()
	if err != nil {
		log.Errorf("housekeeping: error listing hostgroups: %s", err.Error())
		return
	}

	for _, hg := range hostgroups {
		if hg.Vars == nil || hg.Vars[VarCluster] != c.Tag {
			continue
		}

		keep := true

		if hg.Vars[VarOwner] == "" || hg.Vars[VarOwner] == nil {
			log.Warnf("housekeeping: hostgroup '%s' has no owner", hg.Name)
			continue
		}

		namespace, name, err := cache.SplitMetaNamespaceKey(hg.Vars[VarOwner].(string))
		if err != nil {
			log.Errorf("housekeeping: error parsing owner of hostgroup '%s' ('%s'): %s",
				hg.Name, hg.Vars[VarOwner], err.Error())
			continue
		}

		_, err = c.IcingaClient.IcingaV1().HostGroups(namespace).Get(name, metav1.GetOptions{})
		if err != nil {
			if errors.IsNotFound(err) {
				keep = false
			} else {
				log.Errorf("housekeeping: error getting hostgroup resource for '%s/%s': %s",
					namespace, name, err.Error())
				continue
			}
		}

		if !keep {
			log.Infof("housekeeping: deleting obsolete icinga hostgroup '%s'", hg.Name)
			err = c.Icinga.DeleteHostGroup(hg.Name)
			if err != nil {
				log.Errorf("housekeeping: error deleting icinga hostgroup '%s': %s", hg.Name, err.Error())
			}
		}
	}
}

func (c *Controller) IcingaHostHousekeeping() {
	hosts, err := c.Icinga.ListHosts()
	if err != nil {
		log.Errorf("housekeeping: error listing hosts: %s", err.Error())
		return
	}

	for _, h := range hosts {
		if h.Vars == nil || h.Vars[VarCluster] != c.Tag {
			continue
		}

		keep := true

		if h.Vars[VarOwner] == "" || h.Vars[VarOwner] == nil {
			log.Warnf("housekeeping: host '%s' has no owner", h.Name)
			continue
		}

		namespace, name, err := cache.SplitMetaNamespaceKey(h.Vars[VarOwner].(string))
		if err != nil {
			log.Errorf("housekeeping: error parsing owner of host '%s' ('%s'): %s",
				h.Name, h.Vars[VarOwner], err.Error())
			continue
		}

		_, err = c.IcingaClient.IcingaV1().Hosts(namespace).Get(name, metav1.GetOptions{})
		if err != nil {
			if errors.IsNotFound(err) {
				keep = false
			} else {
				log.Errorf("housekeeping: error getting host resource for '%s/%s': %s",
					namespace, name, err.Error())
				continue
			}
		}

		if !keep {
			log.Infof("housekeeping: deleting obsolete icinga host '%s'", h.Name)
			err = c.Icinga.DeleteHost(h.Name)
			if err != nil {
				log.Errorf("housekeeping: error deleting icinga host '%s': %s", h.Name, err.Error())
			}
		}
	}
}

func (c *Controller) IcingaCheckHousekeeping() {
	checks, err := c.Icinga.ListServices()
	if err != nil {
		log.Errorf("housekeeping: error listing checks: %s", err.Error())
		return
	}

	for _, check := range checks {
		if check.Vars == nil || check.Vars[VarCluster] != c.Tag {
			continue
		}

		keep := true

		if check.Vars[VarOwner] == "" || check.Vars[VarOwner] == nil {
			log.Warnf("housekeeping: check '%s' has no owner", check.Name)
			continue
		}

		namespace, name, err := cache.SplitMetaNamespaceKey(check.Vars[VarOwner].(string))
		if err != nil {
			log.Errorf("housekeeping: error parsing owner of check '%s' ('%s'): %s",
				check.Name, check.Vars[VarOwner], err.Error())
			continue
		}

		_, err = c.IcingaClient.IcingaV1().Checks(namespace).Get(name, metav1.GetOptions{})
		if err != nil {
			if errors.IsNotFound(err) {
				keep = false
			} else {
				log.Errorf("housekeeping: error getting check resource for '%s/%s': %s",
					namespace, name, err.Error())
				continue
			}
		}

		if !keep {
			log.Infof("housekeeping: deleting obsolete icinga check '%s'", check.Name)
			err = c.Icinga.DeleteService(check.FullName())
			if err != nil {
				log.Errorf("housekeeping: error deleting icinga check '%s': %s", check.Name, err.Error())
			}
		}
	}
}

func (c *Controller) CrHousekeeping() {

	if c.Mapping.Name() == "hostgroup" {
		hosts, err := c.HostLister.List(labels.Everything())
		if err != nil {
			log.Errorf("error listing hosts: %s", err.Error())
			return
		}

		for _, h := range hosts {
			log.Debugf("[crhousekeeping] checking host '%s/%s'", h.Namespace, h.Name)
			if h.Spec.Vars[VarCluster] != c.Tag {
				log.Debugf("[crhousekeeping] skipping: is tagged for '%s', not us (%s)", h.Spec.Vars[VarCluster], c.Tag)
				continue
			}
			if len(h.OwnerReferences) != 1 {
				log.Debugf("[crhousekeeping] skipping: has %d owners, not the expected 1", len(h.OwnerReferences))
				continue
			}
			or := h.OwnerReferences[0]

			switch or.Kind {
			case "Pod":
				_, err = c.PodLister.Pods(h.Namespace).Get(or.Name)
			case "Deployment":
				_, err = c.DeploymentLister.Deployments(h.Namespace).Get(or.Name)
			case "DaemonSet":
				_, err = c.DaemonSetLister.DaemonSets(h.Namespace).Get(or.Name)
			case "ReplicaSet":
				_, err = c.ReplicaSetLister.ReplicaSets(h.Namespace).Get(or.Name)
			case "StatefulSet":
				_, err = c.StatefulSetLister.StatefulSets(h.Namespace).Get(or.Name)
			case "Node":
				_, err = c.NodeLister.Get(or.Name)
			default:
				err = nil
			}
			if errors.IsNotFound(err) {
				log.Infof("[crhousekeeping] deleting obsolete host '%s/%s' (owner %s '%s/%s' no longer exists)", h.Namespace, h.Name, or.Kind, h.Namespace, or.Name)
				err2 := c.IcingaClient.IcingaV1().Hosts(h.Namespace).Delete(h.Name, &metav1.DeleteOptions{})
				if err2 != nil {
					log.Errorf("[crhousekeeping] error deleting obsolete host '%s/%s': %s", h.Namespace, h.Name, err.Error())
				}
			}
		}
	}
}
