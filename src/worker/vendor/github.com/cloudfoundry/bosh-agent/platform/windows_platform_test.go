// +build windows

package platform_test

import (
	"errors"
	"fmt"
	"io/ioutil"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"sort"
	"strings"
	"sync"
	"syscall"

	"golang.org/x/sys/windows/svc/mgr"

	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"

	fakedpresolv "github.com/cloudfoundry/bosh-agent/infrastructure/devicepathresolver/fakes"
	. "github.com/cloudfoundry/bosh-agent/platform"
	fakecert "github.com/cloudfoundry/bosh-agent/platform/cert/fakes"
	fakeplat "github.com/cloudfoundry/bosh-agent/platform/fakes"
	fakenet "github.com/cloudfoundry/bosh-agent/platform/net/fakes"
	fakestats "github.com/cloudfoundry/bosh-agent/platform/stats/fakes"
	boshsettings "github.com/cloudfoundry/bosh-agent/settings"
	boshdirs "github.com/cloudfoundry/bosh-agent/settings/directories"
	boshlog "github.com/cloudfoundry/bosh-utils/logger"
	boshsys "github.com/cloudfoundry/bosh-utils/system"
	fakesys "github.com/cloudfoundry/bosh-utils/system/fakes"
	fakeuuidgen "github.com/cloudfoundry/bosh-utils/uuid/fakes"
)

var _ = Describe("WindowsPlatform", func() {
	var (
		collector                  *fakestats.FakeCollector
		fs                         *fakesys.FakeFileSystem
		cmdRunner                  *fakesys.FakeCmdRunner
		dirProvider                boshdirs.Provider
		netManager                 *fakenet.FakeManager
		devicePathResolver         *fakedpresolv.FakeDevicePathResolver
		platform                   Platform
		fakeDefaultNetworkResolver *fakenet.FakeDefaultNetworkResolver
		fakeUUIDGenerator          *fakeuuidgen.FakeGenerator
		certManager                *fakecert.FakeManager
		auditLogger                *fakeplat.FakeAuditLogger

		logger boshlog.Logger
	)

	BeforeEach(func() {
		logger = boshlog.NewLogger(boshlog.LevelNone)

		collector = &fakestats.FakeCollector{}
		fs = fakesys.NewFakeFileSystem()
		cmdRunner = fakesys.NewFakeCmdRunner()
		dirProvider = boshdirs.NewProvider("/fake-dir")
		netManager = &fakenet.FakeManager{}
		devicePathResolver = fakedpresolv.NewFakeDevicePathResolver()
		fakeDefaultNetworkResolver = &fakenet.FakeDefaultNetworkResolver{}
		certManager = new(fakecert.FakeManager)
		auditLogger = fakeplat.NewFakeAuditLogger()
		fakeUUIDGenerator = fakeuuidgen.NewFakeGenerator()
	})

	JustBeforeEach(func() {
		platform = NewWindowsPlatform(
			collector,
			fs,
			cmdRunner,
			dirProvider,
			netManager,
			certManager,
			devicePathResolver,
			logger,
			fakeDefaultNetworkResolver,
			auditLogger,
			fakeUUIDGenerator,
		)
	})

	Describe("GetFileContentsFromCDROM", func() {
		It("reads file from D drive", func() {
			fs.WriteFileString("D:/env", "fake-contents")
			contents, err := platform.GetFileContentsFromCDROM("env")
			Expect(err).NotTo(HaveOccurred())
			Expect(contents).To(Equal([]byte("fake-contents")))
		})
	})

	Describe("SetupTmpDir", func() {
		act := func() error {
			return platform.SetupTmpDir()
		}

		It("creates new temp dir", func() {
			err := act()
			Expect(err).NotTo(HaveOccurred())

			fileStats := fs.GetFileTestStat("/fake-dir/data/tmp")
			Expect(fileStats).NotTo(BeNil())
			Expect(fileStats.FileType).To(Equal(fakesys.FakeFileType(fakesys.FakeFileTypeDir)))
		})

		It("returns error if creating new temp dir errs", func() {
			fs.MkdirAllError = errors.New("fake-mkdir-error")

			err := act()
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("fake-mkdir-error"))
		})

		It("sets TMP and TEMP environment variable so that children of this process will use new temp dir", func() {
			err := act()
			Expect(err).NotTo(HaveOccurred())

			fakeTmpDir := filepath.FromSlash("/fake-dir/data/tmp")
			Expect(os.Getenv("TMP")).To(Equal(fakeTmpDir))
			Expect(os.Getenv("TEMP")).To(Equal(fakeTmpDir))
		})

		It("returns error if setting TMPDIR errs", func() {
			// uses os package; no way to trigger err
		})
	})

	Describe("SetupBlobsDir", func() {
		act := func() error {
			return platform.SetupBlobsDir()
		}

		It("creates new temp dir", func() {
			err := act()
			Expect(err).NotTo(HaveOccurred())

			fileStats := fs.GetFileTestStat("/fake-dir/data/blobs")
			Expect(fileStats).NotTo(BeNil())
			Expect(fileStats.FileType).To(Equal(fakesys.FakeFileType(fakesys.FakeFileTypeDir)))
		})

		It("returns error if creating new temp dir errs", func() {
			fs.MkdirAllError = errors.New("fake-mkdir-error")

			err := act()
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("fake-mkdir-error"))
		})
	})

	Describe("SetupDataDir", func() {
		It("creates new temp dir", func() {
			err := platform.SetupDataDir()
			Expect(err).NotTo(HaveOccurred())

			fileStats := fs.GetFileTestStat("/fake-dir/data/sys/log")
			Expect(fileStats).NotTo(BeNil())
			Expect(fileStats.FileType).To(Equal(fakesys.FakeFileType(fakesys.FakeFileTypeDir)))

			fileStats = fs.GetFileTestStat("/fake-dir/sys")
			Expect(fileStats).NotTo(BeNil())
			Expect(fileStats.FileType).To(Equal(fakesys.FakeFileType(fakesys.FakeFileTypeSymlink)))
		})

		It("returns error if creating new temp dir errs", func() {
			fs.MkdirAllError = errors.New("fake-mkdir-error")

			err := platform.SetupDataDir()
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("fake-mkdir-error"))
		})
	})

	Describe("SetupNetworking", func() {
		It("delegates to the NetManager", func() {
			networks := boshsettings.Networks{}

			err := platform.SetupNetworking(networks)
			Expect(err).ToNot(HaveOccurred())

			Expect(netManager.SetupNetworkingNetworks).To(Equal(networks))
		})
	})

	Describe("GetDefaultNetwork", func() {
		It("delegates to the defaultNetworkResolver", func() {
			defaultNetwork := boshsettings.Network{IP: "1.2.3.4"}
			fakeDefaultNetworkResolver.GetDefaultNetworkNetwork = defaultNetwork

			network, err := platform.GetDefaultNetwork()
			Expect(err).ToNot(HaveOccurred())

			Expect(network).To(Equal(defaultNetwork))
		})
	})

	Describe("SetTimeWithNtpServers", func() {
		It("sets time with ntp servers", func() {
			servers := []string{"0.north-america.pool.ntp.org", "1.north-america.pool.ntp.org"}
			platform.SetTimeWithNtpServers(servers)

			Expect(len(cmdRunner.RunCommands)).To(Equal(6))
			Expect(cmdRunner.RunCommands[0]).To(ContainElement(ContainSubstring("new-netfirewallrule")))
			Expect(cmdRunner.RunCommands[1]).To(ContainElement(ContainSubstring("stop")))
			ntpServers := strings.Join(servers, " ")
			Expect(cmdRunner.RunCommands[2]).To(ContainElement(ContainSubstring(ntpServers)))
			Expect(cmdRunner.RunCommands[3]).To(ContainElement(ContainSubstring("start")))
			Expect(cmdRunner.RunCommands[4]).To(ContainElement(ContainSubstring("/update")))
			Expect(cmdRunner.RunCommands[5]).To(ContainElement(ContainSubstring("/resync")))
		})

		It("sets time with ntp servers is noop when no ntp server provided", func() {
			platform.SetTimeWithNtpServers([]string{})
			Expect(len(cmdRunner.RunCommands)).To(Equal(0))
		})
	})

	Describe("DeleteARPEntryWithIP", func() {
		It("cleans the arp entry for the given ip", func() {
			err := platform.DeleteARPEntryWithIP("1.2.3.4")
			deleteArpEntry := []string{"arp", "-d", "1.2.3.4"}
			Expect(cmdRunner.RunCommands[0]).To(Equal(deleteArpEntry))
			Expect(err).ToNot(HaveOccurred())
		})

		It("fails if arp command fails", func() {
			result := fakesys.FakeCmdResult{
				Error:      errors.New("failure"),
				ExitStatus: 1,
				Stderr:     "",
				Stdout:     "",
			}
			cmdRunner.AddCmdResult("arp -d 1.2.3.4", result)

			err := platform.DeleteARPEntryWithIP("1.2.3.4")

			Expect(err).To(HaveOccurred())
		})
	})

	Describe("SaveDNSRecords", func() {
		var dnsRecords boshsettings.DNSRecords

		BeforeEach(func() {
			dnsRecords = boshsettings.DNSRecords{
				Records: [][2]string{
					{"fake-ip0", "fake-name0"},
					{"fake-ip1", "fake-name1"},
				},
			}
		})

		It("writes the new DNS records in '/etc/hosts'", func() {
			err := platform.SaveDNSRecords(dnsRecords, "fake-hostname")
			Expect(err).ToNot(HaveOccurred())

			windir := os.Getenv("windir")
			hostsFileContents, err := fs.ReadFile(windir + "\\System32\\Drivers\\etc\\hosts")
			Expect(err).ToNot(HaveOccurred())

			Expect(hostsFileContents).Should(MatchRegexp("fake-ip0\\s+fake-name0\\n"))
			Expect(hostsFileContents).Should(MatchRegexp("fake-ip1\\s+fake-name1\\n"))
		})

		It("renames intermediary /etc/hosts-<uuid> atomically to /etc/hosts", func() {
			err := platform.SaveDNSRecords(dnsRecords, "fake-hostname")
			Expect(err).ToNot(HaveOccurred())

			Expect(fs.RenameError).ToNot(HaveOccurred())

			// Use '/Windows' to make the fakefilesystem happy...
			Expect(len(fs.RenameOldPaths)).To(Equal(1))
			Expect(fs.RenameOldPaths).To(ContainElement("/Windows/System32/Drivers/etc/hosts-fake-uuid-0"))

			Expect(len(fs.RenameNewPaths)).To(Equal(1))
			Expect(fs.RenameNewPaths).To(ContainElement("/Windows/System32/Drivers/etc/hosts"))
		})
	})

	Describe("GetHostPublicKey", func() {
		var previous func() error

		BeforeEach(func() {
			previous = SetSSHEnabled(func() error { return nil })
		})

		AfterEach(func() {
			SetSSHEnabled(previous)
		})

		const ExpPublicKey = "PUBLIC RSA KEY"

		setupHostKeys := func(drive string) {
			if drive == "" {
				drive = "C:"
			}
			drive += "\\"

			dirname := filepath.Join(drive, "Program Files", "OpenSSH")
			fs.MkdirAll(dirname, 0744)
			var keyTypes = []string{
				"dsa",
				"ecdsa",
				"ed25519",
				"rsa",
			}
			for _, s := range keyTypes {
				name := fmt.Sprintf("ssh_host_%s_key", s)
				path := filepath.Join(dirname, name)

				fs.WriteFileString(path, fmt.Sprintf("PRIVATE %s KEY", strings.ToUpper(s)))
				path += ".pub"
				fs.WriteFileString(path, fmt.Sprintf("PUBLIC %s KEY", strings.ToUpper(s)))
			}
		}

		It("reads the host RSA key", func() {
			setupHostKeys(os.Getenv("SYSTEMDRIVE"))
			key, err := platform.GetHostPublicKey()
			Expect(err).ToNot(HaveOccurred())
			Expect(key).To(Equal(ExpPublicKey))
		})

		It("reads the host key stored in %SYSTEMDRIVE%\\Program Files\\OpenSSH", func() {
			oldSys := os.Getenv("SYSTEMDRIVE")
			defer os.Setenv("SYSTEMDRIVE", oldSys)
			newSys := "K:"
			os.Setenv("SYSTEMDRIVE", newSys)

			setupHostKeys(newSys)

			key, err := platform.GetHostPublicKey()
			Expect(err).ToNot(HaveOccurred())
			Expect(key).To(Equal(ExpPublicKey))
		})

		It("fails if the sshd daemon is not running", func() {
			setupHostKeys(os.Getenv("SYSTEMDRIVE"))

			previous := SetSSHEnabled(func() error { return errors.New("test") })
			defer SetSSHEnabled(previous)

			_, err := platform.GetHostPublicKey()
			Expect(err).To(HaveOccurred())
		})
	})

	Describe("BOSH SSH Commands", func() {
		const testUsername = boshsettings.EphemeralUserPrefix + "test_abc123"

		var (
			platform Platform

			// We're doing this for real - no fakes!
			logger      = boshlog.NewLogger(boshlog.LevelNone)
			fs          = boshsys.NewOsFileSystem(logger)
			dirProvider = boshdirs.NewProvider("/fake-dir")
			cmdRunner   = boshsys.NewExecCmdRunner(logger)

			deleteUserOnce sync.Once
		)

		BeforeEach(func() {
			deleteUserOnce.Do(func() {
				DeleteUserProfile(testUsername)
			})

			var (
				collector                  = &fakestats.FakeCollector{}
				netManager                 = &fakenet.FakeManager{}
				devicePathResolver         = fakedpresolv.NewFakeDevicePathResolver()
				fakeDefaultNetworkResolver = &fakenet.FakeDefaultNetworkResolver{}
				certManager                = new(fakecert.FakeManager)
				auditLogger                = fakeplat.NewFakeAuditLogger()
				fakeUUIDGenerator          = fakeuuidgen.NewFakeGenerator()
			)
			platform = NewWindowsPlatform(
				collector,
				fs,
				cmdRunner,
				dirProvider,
				netManager,
				certManager,
				devicePathResolver,
				logger,
				fakeDefaultNetworkResolver,
				auditLogger,
				fakeUUIDGenerator,
			)
		})

		userExists := func(name string) error {
			_, _, t, err := syscall.LookupSID("", name)
			if err != nil {
				return err
			}
			if t != syscall.SidTypeUser {
				return fmt.Errorf("not a user sid: %s", name)
			}
			return nil
		}

		AfterEach(func() {
			DeleteUserProfile(testUsername)
			Expect(userExists(testUsername)).ToNot(Succeed())
		})

		It("can create a user with Admin privileges", func() {
			Expect(platform.CreateUser(testUsername, "")).To(Succeed())
			Expect(userExists(testUsername)).To(Succeed())

			cmd := exec.Command("NET.exe", "LOCALGROUP", "Administrators")
			out, err := cmd.CombinedOutput()
			Expect(err).ToNot(HaveOccurred())
			Expect(string(out)).To(ContainSubstring(testUsername))
		})

		sshdServiceIsInstalled := func() bool {
			m, err := mgr.Connect()
			if err != nil {
				return false
			}
			defer m.Disconnect()
			s, err := m.OpenService("sshd")
			if err != nil {
				return false
			}
			s.Close()
			return true
		}

		It("can insert public keys into the users .ssh\\authorized_keys file", func() {
			if !sshdServiceIsInstalled() {
				Skip("This test requires the SSHD service to be installed")
			}

			keys := []string{
				"KEY_1",
				"KEY_2",
				"KEY_3",
			}
			Expect(platform.CreateUser(testUsername, "")).To(Succeed())
			Expect(userExists(testUsername)).To(Succeed())

			Expect(platform.SetupSSH(keys, testUsername)).To(Succeed())

			homedir, err := UserHomeDirectory(testUsername)
			Expect(err).To(Succeed())

			keyPath := filepath.Join(homedir, ".ssh", "authorized_keys")
			b, err := ioutil.ReadFile(keyPath)
			Expect(err).To(Succeed())

			content := strings.TrimSpace(string(b))
			for i, line := range strings.Split(content, "\n") {
				line = strings.TrimSpace(line)
				Expect(line).To(Equal(keys[i]))
			}

			out, err := exec.Command("icacls.exe", keyPath).CombinedOutput()
			Expect(err).To(Succeed())
			Expect(strings.ToUpper(string(out))).To(ContainSubstring("NT SERVICE\\SSHD:(R)"))
		})

		It("can delete a users matching a regex", func() {
			Expect(platform.CreateUser(testUsername, "")).To(Succeed())
			Expect(userExists(testUsername)).To(Succeed())

			homedir, err := UserHomeDirectory(testUsername)
			Expect(err).To(Succeed())

			// Regex taken from: github.com/cloudfoundry/bosh-cli/director/ssh.go
			//
			const regex = "^" + testUsername
			Expect(platform.DeleteEphemeralUsersMatching(regex)).To(Succeed())
			Expect(userExists(testUsername)).ToNot(Succeed())

			_, err = os.Stat(homedir)
			Expect(err).To(HaveOccurred())
		})
	})
})

var _ = Describe("Windows Syscalls and Helper functions", func() {
	It("Generates valid Windows passwords", func() {
		// 100,000 iterations takes about 140ms to run in a VM.
		for i := 0; i < 100000; i++ {
			s, err := RandomPassword()
			Expect(err).To(BeNil())
			Expect(s).To(HaveLen(14))
			Expect(s).ToNot(ContainSubstring("/"))
			Expect(ValidWindowsPassword(s)).To(BeTrue())
		}
	})

	expectedUserNames := func() ([]string, error) {
		cmd := exec.Command("PowerShell", "-Command",
			"Get-WmiObject -Class Win32_UserAccount | foreach { $_.Name }")

		out, err := cmd.CombinedOutput()
		if err != nil {
			return nil, err
		}
		exp := strings.Fields(string(out))
		sort.Strings(exp)
		return exp, nil
	}

	It("Lists local user accounts", func() {
		exp, err := expectedUserNames()
		Expect(err).To(Succeed())

		names, err := LocalAccountNames()
		Expect(err).To(Succeed())

		sort.Strings(names)
		Expect(names).To(Equal(exp))
	})

	It("Does not fail in a tight loop", func() {
		var wg sync.WaitGroup
		numCPU := runtime.NumCPU()
		if numCPU > 4 {
			numCPU = 4
		}
		for i := 0; i < numCPU; i++ {
			wg.Add(1)
			go func() {
				defer wg.Done()
				for i := 0; i < 5000; i++ {
					names, err := LocalAccountNames()
					Expect(err).To(Succeed())
					Expect(names).ToNot(HaveLen(0))
				}
			}()
		}
		wg.Wait()
	})
})
