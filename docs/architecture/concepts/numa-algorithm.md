# Algorithm for NUMA-aware placement of VirtualMachines

- [Preparation Steps](#preparation-steps)
- [Core Logic](#core-logic)

## Preparation Steps

1. Define an example `Machine` object with the configuration of 8G and 4 Cores and reconcile it.

1. Build the `CPUTopology` map once at the start, which creates a Go-map representation of the CPU topology on the host, with `map[int][]int`.

    ```text
    Example:
    CPUTopology {
    node-id-0: [0 1 2 3 4 5 6 7 8 9 10 11 24 25 26 27 28 29 30 31 32 33 34 35],
    node-id-1: [12 13 14 15 16 17 18 19 20 21 22 23 36 37 38 39 40 41 42 43 44 45 46 47],
    }
    ```

1. In every new `Machine` reconciliation, analyze the host machine.

    - Check and store the default huge page size.
    - Check if 8G is available in total on the host machine to be allocated in terms of pre-allocated hugepages.

1. Create the map called `pins`, which represents the number of VMs assigned to each of the CPU cores, sorted from least to highest.

    ```text
    Example: pins = [0:1 1:2 2:2 ... 33:3 34:3 35:4]
    ```

1. Create a sorted map from `CPUTopology`, which is an array of CPU cores per NUMA zones, sorted from least to highest number of VMs assigned to the CPU cores.

    ```text
    Example:
    Sorted CPUTopology {
    node-id-0: [0 1 2 3 4 5 6 7 8 9 10 11 24 25 26 27 28 29 30 31 32 33 34 35],
    node-id-1: [12 13 14 15 16 17 18 19 20 21 22 23 36 37 38 39 40 41 42 43 44 45 46 47],
    }
    ```

## Core Logic

1. Choose the nodeset with the most available Memory, either 0 or 1.

    - If neither of the nodesets have enough memory available, allocate the remaining memory from both nodesets.

    ```text
    Example: AllocatedMemory - [0:6G 1:2G]
    ```

    - Based on the proportion of memory, determine the number of CPUs to choose from the CPUTopology from both NodeSets.

    ```text
    Example:
    If the proportion of memory is 75% from NodeSet0 and 25% from NodeSet1, and there are 4 CPUs available in total, then 3 CPUs will be allocated from NodeSet0 and 1 CPU will be allocated from NodeSet1. Choose ceiling values in the case of non-integer values.
    ```

2. Create the domain XML from the information above.

    ```xml
    <memoryBacking>
        <hugepages/>
    </memoryBacking>
    <vcpu placement='static'>4</vcpu>
    <cputune>
        <vcpupin vcpu='0' cpuset='0'/>
        <vcpupin vcpu='1' cpuset='1'/>
        <vcpupin vcpu='2' cpuset='2'/>
        <vcpupin vcpu='3' cpuset='15'/>
    </cputune>

    <numatune>
        <memory mode='strict' nodeset='0-   1'/>
        <memnode cellid='0' mode='strict' nodeset='0'/>
        <memnode cellid='1' mode='strict' nodeset='1'/>
    </numatune>

    <cpu mode='custom' match='exact' check='full'>
        <numa>
        <cell id='0' cpus='0-2' memory='6G' unit='KiB'/>
        <cell id='1' cpus='3' memory='2G' unit='KiB'/>
        </numa>
    </cpu>
    ```
