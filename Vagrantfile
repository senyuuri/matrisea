Vagrant.configure("2") do |config|
    config.vm.network "private_network", type: "dhcp"
    config.vm.define "dev" do |dev|
        dev.vm.box = "ubuntu/focal64"
        dev.vm.disk :disk, size: "200GB", primary: true
        dev.vm.provider "virtualbox" do |vb|
            vb.customize [
                'modifyvm', :id, 
                '--nested-hw-virt', 'on'
                "--paravirtprovider", "kvm",
            ]
            vb.cpus = 8
            vb.memory = 8192
        end
        dev.vm.provision "docker" 
        dev.vm.provision "shell", inline: "curl -L https://github.com/docker/compose/releases/download/v2.5.0/docker-compose-$(uname -s)-$(uname -m) -o /usr/bin/docker-compose"
        dev.vm.provision "shell", inline: "chmod +x /usr/bin/docker-compose"
        dev.vm.provision "shell", inline: "REAL_USER=vagrant /bin/sh -c /vagrant/setup.sh"
    end
end