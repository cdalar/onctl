package cloud

import (
	"context"
	"log"
	"math"
	"time"

	"github.com/vmware/govmomi"
	"github.com/vmware/govmomi/find"
	"github.com/vmware/govmomi/object"
	"github.com/vmware/govmomi/vim25/mo"
	"github.com/vmware/govmomi/vim25/types"
)

type ProviderVsphere struct {
	Client *govmomi.Client
}

func (p ProviderVsphere) List() (VmList, error) {
	log.Println("[DEBUG] List VMs in vSphere")

	ctx := context.TODO()
	finder := find.NewFinder(p.Client.Client, false)

	datacenter, err := finder.DefaultDatacenter(ctx)
	if err != nil {
		return VmList{}, err
	}
	log.Println("[DEBUG] Datacenter: ", datacenter.Name())
	finder.SetDatacenter(datacenter)

	vms, err := finder.VirtualMachineList(ctx, "*")
	if err != nil {
		return VmList{}, err
	}

	cloudList := make([]Vm, 0, len(vms))
	for _, vm := range vms {
		log.Println("[DEBUG] Processing VM: ", vm.Name())
		vmProps := mo.VirtualMachine{}
		err := vm.Properties(ctx, vm.Reference(), []string{"summary", "config"}, &vmProps)
		if err != nil {
			log.Println("[ERROR] Failed to get properties for VM:", err)
			continue
		}
		// if vmProps.Summary.Config.Annotation != "Owner=onctl" {
		// 	continue
		// }
		cloudList = append(cloudList, mapVsphereServer(vmProps))
	}

	return VmList{List: cloudList}, nil
}

func (p ProviderVsphere) Deploy(Vm) (Vm, error) {
	log.Println("[DEBUG] Deploying VM in vSphere")
	ctx := context.TODO()
	// Create Finder to locate objects in vCenter
	finder := find.NewFinder(p.Client.Client, true)

	// Find datacenter
	datacenter, err := finder.DefaultDatacenter(ctx)
	if err != nil {
		log.Fatalln("Failed to find default datacenter:", err)
	}
	finder.SetDatacenter(datacenter)

	resourcePools, err := finder.ResourcePoolList(ctx, "*")
	if err != nil {
		log.Fatalln("Failed to list resource pools:", err)
	}

	for _, rp := range resourcePools {
		log.Println("[DEBUG] Resource Pool:", rp.InventoryPath)
	}

	// Find resource pool
	// resourcePool, err := finder.DefaultResourcePool(ctx)
	resourcePool, err := finder.ResourcePool(ctx, "/DC0/host/DC0_H0/Resources")
	log.Println("[DEBUG] Resource Pool: ", resourcePool)
	if err != nil {
		log.Fatalln("Failed to find default resource pool:", err)
	}

	// Find datastore
	// datastore, err := finder.DefaultDatastore(ctx)
	// if err != nil {
	// 	log.Fatalln("Failed to find default datastore:", err)
	// }

	// Create a folder for the new VM
	dcFolders, err := datacenter.Folders(ctx)
	if err != nil {
		log.Fatalln("Failed to get datacenter folders:", err)
	}
	folder := dcFolders.VmFolder

	// Define VM config
	vmConfigSpec := types.VirtualMachineConfigSpec{
		Name:     "test-vm",    // Name of the new VM
		GuestId:  "otherGuest", // Guest OS type
		NumCPUs:  2,            // Number of CPUs
		MemoryMB: 4096,         // Memory in MB
		DeviceChange: []types.BaseVirtualDeviceConfigSpec{
			// Add a virtual disk
			&types.VirtualDeviceConfigSpec{
				Operation: types.VirtualDeviceConfigSpecOperationAdd,
				Device: &types.VirtualDisk{
					VirtualDevice: types.VirtualDevice{
						Key: -1000,
						Backing: &types.VirtualDiskFlatVer2BackingInfo{
							DiskMode:        string(types.VirtualDiskModePersistent),
							ThinProvisioned: types.NewBool(true),
						},
					},
					CapacityInKB: 16 * 1024 * 1024, // 16 GB disk
				},
			},
		},
	}

	// poolRef := resourcePool.Reference()
	// datastoreRef := datastore.Reference()

	// relocateSpec := types.VirtualMachineRelocateSpec{
	// 	Pool:      &poolRef,
	// 	Datastore: &datastoreRef,
	// }

	// // Create VM clone spec
	// cloneSpec := types.VirtualMachineCloneSpec{
	// 	Location: relocateSpec,
	// 	PowerOn:  false,
	// }

	// Create the VM
	task, err := folder.CreateVM(ctx, vmConfigSpec, resourcePool, (*object.HostSystem)(dcFolders.HostFolder))
	if err != nil {
		log.Fatalln("Failed to create VM:", err)
	}

	log.Println("Waiting for task completion...")
	taskInfo, err := task.WaitForResult(ctx, nil)
	if err != nil {
		log.Fatalln("Task failed:", err)
	}

	log.Printf("VM created successfully: %v\n", taskInfo.Result)
	return mapVsphereServer(mo.VirtualMachine{}), nil
}

func (p ProviderVsphere) Destroy(Vm) error {
	log.Println("[DEBUG] Destroying VM in vSphere")
	return nil
}

func (p ProviderVsphere) CreateSSHKey(publicKeyFile string) (keyID string, err error) {
	log.Println("[DEBUG] Creating SSH key in vSphere")
	return "", nil
}

func (p ProviderVsphere) SSHInto(serverName string, port int, privateKey string) {

}

func (p ProviderVsphere) GetByName(serverName string) (Vm, error) {
	log.Println("[DEBUG] Getting VM by name in vSphere")
	return Vm{}, nil
}

// mapVsphereServer maps a vSphere VirtualMachine to Vm
func mapVsphereServer(vm mo.VirtualMachine) Vm {
	summary := vm.Summary
	config := summary.Config
	runtime := summary.Runtime

	uptime := time.Since(*runtime.BootTime)
	hourlyCost := calculateHourlyCost(config.CpuReservation, int64(config.MemoryReservation))
	acculumatedCost := math.Round(hourlyCost*uptime.Hours()*10000) / 10000
	costPerMonth := hourlyCost * 24 * 30

	privateIP := "N/A"
	if summary.Guest.IpAddress != "" {
		privateIP = summary.Guest.IpAddress // assuming first IP is private
	}

	return Vm{
		Provider:  "vsphere",
		ID:        vm.Reference().Value,
		Name:      summary.Config.Name,
		IP:        summary.Guest.IpAddress,
		PrivateIP: privateIP,
		Type:      config.GuestFullName,
		Status:    string(runtime.PowerState),
		CreatedAt: *runtime.BootTime,
		// Location:  config.DatastoreUrl[0].Datastore, // example, adjust based on your location metadata
		Cost: CostStruct{
			Currency:        "USD", // Assuming USD
			CostPerHour:     hourlyCost,
			CostPerMonth:    costPerMonth,
			AccumulatedCost: acculumatedCost,
		},
	}
}

func calculateHourlyCost(cpuReservation int32, memoryReservation int64) float64 {
	// Simplified cost calculation based on reservation
	cpuCost := float64(cpuReservation) * 0.05              // Assume $0.05 per reserved MHz
	memoryCost := float64(memoryReservation) * 0.01 / 1024 // Assume $0.01 per reserved GB
	return cpuCost + memoryCost
}
