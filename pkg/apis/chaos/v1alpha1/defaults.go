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
