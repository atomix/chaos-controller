/*
 * Copyright (c) 2019-present Open Networking Foundation
 *
 * Licensed under the Apache License, Version 2.0 (the "License");
 * you may not use this file except in compliance with the License.
 * You may obtain a copy of the License at
 *
 *     http://www.apache.org/licenses/LICENSE-2.0
 *
 * Unless required by applicable law or agreed to in writing, software
 * distributed under the License is distributed on an "AS IS" BASIS,
 * WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
 * See the License for the specific language governing permissions and
 * limitations under the License.
 */

package v1alpha1

// SetDefaults sets the default values for the ChaosMonkey spec.
func SetDefaults(monkey *ChaosMonkey) {
	minute := int64(60)
	one := int(1)
	zero := float64(0)
	oneThousand := int64(1000)

	if monkey.Spec.RateSeconds == nil {
		monkey.Spec.RateSeconds = &minute
	}
	if monkey.Spec.PeriodSeconds == nil {
		monkey.Spec.PeriodSeconds = &minute
	}
	if monkey.Spec.Jitter == nil {
		monkey.Spec.Jitter = &zero
	}
	if monkey.Spec.Crash != nil {
		if monkey.Spec.Crash.CrashStrategy.Type == "" {
			monkey.Spec.Crash.CrashStrategy.Type = CrashContainer
		}
	} else if monkey.Spec.Partition != nil {
		if monkey.Spec.Partition.PartitionStrategy.Type == "" {
			monkey.Spec.Partition.PartitionStrategy.Type = PartitionIsolate
		}
	}
	if monkey.Spec.Stress != nil {
		if monkey.Spec.Stress.StressStrategy.Type == "" {
			monkey.Spec.Stress.StressStrategy.Type = StressAll
		}
		if monkey.Spec.Stress.IO != nil {
			if monkey.Spec.Stress.IO.Workers == nil {
				monkey.Spec.Stress.IO.Workers = &one
			}
		}
		if monkey.Spec.Stress.CPU != nil {
			if monkey.Spec.Stress.CPU.Workers == nil {
				monkey.Spec.Stress.CPU.Workers = &one
			}
		}
		if monkey.Spec.Stress.Memory != nil {
			if monkey.Spec.Stress.Memory.Workers == nil {
				monkey.Spec.Stress.Memory.Workers = &one
			}
		}
		if monkey.Spec.Stress.HDD != nil {
			if monkey.Spec.Stress.HDD.Workers == nil {
				monkey.Spec.Stress.HDD.Workers = &one
			}
		}
		if monkey.Spec.Stress.Network != nil {
			if monkey.Spec.Stress.Network.LatencyMilliseconds == nil {
				monkey.Spec.Stress.Network.LatencyMilliseconds = &oneThousand
			}
		}
	}
}
