# GSI Patchset
This folder contains a set of patches to can be applied on AOSP Generic System Images(GSI) to achieve certain system modification goals. 

Our philosophy:
- Use GSI to ensure everyone's having an idential image to base their work on
- Use git patchset to transparently articulate what changes have been applied so advanced users can inspect or even cherry pick features they like. No more suspicious pre-built images in a `.zip` that you have no idea what's inside.
- The patchset should closely track upstream GSI branches so it can be always applied to the latest GSI branch (and there's only ONE GSI per Android major release). Similar to [how `android-mainline` track its differences from the Linux mainline kernel](https://android.googlesource.com/kernel/common-patches/+/refs/heads/master/android-mainline/).


### `aosp_10_gsi_gapps` - Android 10 GSI with OpenGapps
1. Download AOSP source code and pick `android10_gsi` branch (https://source.android.com/setup/build/downloading)
2. Setup and sync OpenGapps repos and git lfs pull, as per instructions on https://github.com/opengapps/aosp_build
3. Patch AOSP
    ```bash
    cd {your-aosp-root}/device/google/cuttlefish
    git am {matrisea-root}/patches/aosp_10_gsi_gapps/*.patch
    ```

4. (For x86/x64 build) Patch OpenGapps for known x86 compatibility issues
    ```bash
    cd {your-aosp-root}/vendor/opengapps/build
    git am {matrisea-root}/patches/opengapps_10_x86_64/*.patch
    ```