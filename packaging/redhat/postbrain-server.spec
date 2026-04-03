Name:           postbrain-server
Version:        0.0.0
Release:        1%{?dist}
Summary:        Postbrain server daemon

License:        Apache-2.0
URL:            https://github.com/simplyblock/postbrain
Source0:        %{name}-%{version}.tar.gz

BuildRequires:  golang

%description
Postbrain server provides persistent memory and knowledge APIs for coding agents.

%prep
%autosetup

%build
make build

%install
install -D -m 0755 postbrain %{buildroot}%{_bindir}/postbrain
install -D -m 0644 config.example.yaml %{buildroot}%{_sysconfdir}/postbrain/config.yaml

%files
%license LICENSE
%doc README.md
%{_bindir}/postbrain
%config(noreplace) %{_sysconfdir}/postbrain/config.yaml

%changelog
* Fri Apr 03 2026 Postbrain Team <opensource@simplyblock.io> - 0.0.0-1
- Initial server package scaffold
