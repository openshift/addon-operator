FROM scratch

ADD ./hack/hypershift/package/hc/*.yaml* /package/
ADD ./hack/hypershift/package/hcp/*.yaml* /package/
ADD ./hack/hypershift/package/*.yaml /package/
