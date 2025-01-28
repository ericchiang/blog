+++
title = "Confidential Compute and GPUs"
date = "2025-01-27"
+++

Recently, I’ve had a few conversations about [NVIDIA Confidential Compute](https://developer.nvidia.com/blog/confidential-computing-on-h100-gpus-for-secure-and-trustworthy-ai/), usually in the context of startups trying to sell security products to AI companies.

The pitch generally looks like this: Companies are protective of their weights and their data. We should be able to train and/or run models on GPUs securely using attestation primitives. In the same way that we might store a private key in an HSM, surely we can design a similar construct for AI.
## On confidential compute
“Confidential compute” encompasses a few different technologies. Trusted Execution Environments (TTEs), encrypted memory, and hardware attestation.

A TEE runs small bits of code without an OS, directly on the CPU, with the hardware verifying the executable against an allowed list of hashes for integrity. Skipping the OS provides an easy-to-attest link between the hardware and the code, but forgoes fancy programming features like threads. As such, TEEs are used for low-level credential managers like firmware Trusted Platform Modules (fTPMs), rather than a privileged space to run application logic.

For encrypted memory, as a host boots, or a hypervisor initializes a VM, it coordinates with the CPU to generate encryption keys for memory or virtual pages. Decryption happens transparently as the CPU transitions to execute host or guest instructions, so everything looks the same to the OS. “Confidential VMs” are a branding of this setup, where the guest memory is encrypted through the hypervisor in a way that the host can’t tamper with.

“[Confidential GPUs](https://cacm.acm.org/practice/creating-the-first-confidential-gpus/)” is the integration of the GPU and encrypted memory. Custom drivers facilitate the transfer of data between confidential memory regions and the GPU, and the memory management unit marks GPU regions as associated with specific VMs. Similar to a confidential VM, all encryption and decryption is done transparently. [_“The goal is to have the existing code and kernels from users work without changes…”_](https://cacm.acm.org/practice/creating-the-first-confidential-gpus/#sec5). If you’re running in a VM, you can access any part of GPU memory that’s assigned to that VM. 

The vast majority of users enable these features through cloud provider APIs or hardware configuration without further verification. A confidential VM can technically [challenge hardware](https://www.amd.com/content/dam/amd/en/documents/developer/lss-snp-attestation.pdf) to attest its memory is protected against the host. But doing so is only important if you don’t trust your hypervisor (e.g. your Cloud provider) to enable this feature in the first place.
## What’s this GPU running?
So, we’ve got a secure GPU, secure CPU, and secure memory. Time for some secure AI!

Well…

Attesting a GPU or the encryption state of RAM doesn’t imply that the code coordinating the GPU is trustworthy. Is your GPU being asked to do AI inference, or mine crypto? If we’re protecting against tampering with the GPU, CPU, or memory, how do we bootstrap similar trust in the storage that’s providing code to run?

This is where more traditional OS security solutions, such as Secure Boot, come into play. Secure Boot allows [signatures over](https://0pointer.net/blog/brave-new-trusted-boot-world.html) the kernel, initrd, and boot stubs, which are then verified by firmware configuration. On laptops, this is a mechanism that prevents someone with physical access to your device from messing with your unencrypted boot region.

The TEE has a role to play here. Since we can’t just trust a machine telling us “yep, Secure Boot is enabled,” devices can rely on an fTPM hosted by a TEE to measure and attest the boot configuration. The TPM interface can also be used to guard secrets like [disk encryption keys](https://learn.microsoft.com/en-us/windows/security/operating-system-security/data-protection/bitlocker/).

TPMs for physical machines are ubiquitous since Windows started requiring one, and virtual TPMs for VMs on Cloud providers are actually quite common. Unfortunately in the Cloud, you’re back to trusting the hypervisor to supply a secure vTPM implementation, which is what confidential compute is try to avoid in the first place. There is [research by IBM](https://arxiv.org/pdf/2303.16463) on verifiable vTPM backed by hardware, only requiring a couple kernel and hypervisor patches.
## Securing the data
Okay, we’ve hardened our OS, attested a number of fidgety components through hardware, and convinced a Cloud provider to apply patches from an academic paper. All to run an encrypted, verified VM. Are we secure yet?

To date, Apple’s [Private Cloud Compute](https://security.apple.com/blog/private-cloud-compute/) is one of the more public attempts to provide cohesive data guarantees for AI inference. The Secure Enclave (Apple’s TEE equivalent) and Secure Boot are both mentioned early in the whitepaper, but are foundational instead of being used directly to attest the system to users.

Hardware security doesn’t mean much without broader controls around how data can move in and out of the system. While Private Cloud Compute leverages cryptographic schemes and provides a binary audit log for researchers, I’d describe a significant amount of the controls as pragmatic production hardening. Code signing, removal of remote shells, using a memory safe language, not storing the user query in a database, whipping the disk between reboots, a minimal base OS. 

To put it another way: what’s the point of encrypted RAM if your application chooses to do something unsafe? Hardware controls don't exist in a vacuum and are no guarantee that your software handles data in a secure way.

Ultimately, a user can’t trust Apple’s privacy claims because of a hardware attestation. There’s no signed receipt from the Secure Enclave saying: “I do attest that this query was private.” You have to trust the software and operational model, as much as the hardware.

## All for what?

As soon as you suppose that a low-level component might be lying to you, this often creates more questions than answers. If we need to attest our GPU, why do we trust our OS? If we’ve measured Secure Boot configuration, isn’t it also important to measure the firmware? Surely TEEs are secure and no one would ever try to [fuzz one](https://www.usenix.org/conference/usenixsecurity22/presentation/cloosters)?

At some point, isn't it just easier to pretend our hardware is trustworthy?

If you’re building a phone or game console, doing this kind of hardware security is part of the deal. For a startup, a security control is a hard sell if a prerequisite is “manage and attest all software and hardware in the boot chain.” All to require the same application security work your company already needs to do.

